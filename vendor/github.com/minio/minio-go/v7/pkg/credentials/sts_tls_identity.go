// MinIO Go Library for Amazon S3 Compatible Cloud Storage
// Copyright 2021 MinIO, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package credentials

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// CertificateIdentityOption is an optional AssumeRoleWithCertificate
// parameter - e.g. a custom HTTP transport configuration or S3 credental
// livetime.
type CertificateIdentityOption func(*STSCertificateIdentity)

// CertificateIdentityWithTransport returns a CertificateIdentityOption that
// customizes the STSCertificateIdentity with the given http.RoundTripper.
func CertificateIdentityWithTransport(t http.RoundTripper) CertificateIdentityOption {
	return CertificateIdentityOption(func(i *STSCertificateIdentity) {
		if i.Client == nil {
			i.Client = &http.Client{}
		}
		i.Client.Transport = t
	})
}

// CertificateIdentityWithExpiry returns a CertificateIdentityOption that
// customizes the STSCertificateIdentity with the given livetime.
//
// Fetched S3 credentials will have the given livetime if the STS server
// allows such credentials.
func CertificateIdentityWithExpiry(livetime time.Duration) CertificateIdentityOption {
	return CertificateIdentityOption(func(i *STSCertificateIdentity) { i.S3CredentialLivetime = livetime })
}

// A STSCertificateIdentity retrieves S3 credentials from the MinIO STS API and
// rotates those credentials once they expire.
type STSCertificateIdentity struct {
	Expiry

	// Optional http Client to use when connecting to MinIO STS service.
	// (overrides default client in CredContext)
	Client *http.Client

	// STSEndpoint is the base URL endpoint of the STS API.
	// For example, https://minio.local:9000
	STSEndpoint string

	// S3CredentialLivetime is the duration temp. S3 access
	// credentials should be valid.
	//
	// It represents the access credential livetime requested
	// by the client. The STS server may choose to issue
	// temp. S3 credentials that have a different - usually
	// shorter - livetime.
	//
	// The default livetime is one hour.
	S3CredentialLivetime time.Duration

	// Certificate is the client certificate that is used for
	// STS authentication.
	Certificate tls.Certificate

	// Optional, used for token revokation
	TokenRevokeType string
}

// NewSTSCertificateIdentity returns a STSCertificateIdentity that authenticates
// to the given STS endpoint with the given TLS certificate and retrieves and
// rotates S3 credentials.
func NewSTSCertificateIdentity(endpoint string, certificate tls.Certificate, options ...CertificateIdentityOption) (*Credentials, error) {
	identity := &STSCertificateIdentity{
		STSEndpoint: endpoint,
		Certificate: certificate,
	}
	for _, option := range options {
		option(identity)
	}
	return New(identity), nil
}

// RetrieveWithCredContext is Retrieve with cred context
func (i *STSCertificateIdentity) RetrieveWithCredContext(cc *CredContext) (Value, error) {
	if cc == nil {
		cc = defaultCredContext
	}

	stsEndpoint := i.STSEndpoint
	if stsEndpoint == "" {
		stsEndpoint = cc.Endpoint
	}
	if stsEndpoint == "" {
		return Value{}, errors.New("STS endpoint unknown")
	}

	endpointURL, err := url.Parse(stsEndpoint)
	if err != nil {
		return Value{}, err
	}
	livetime := i.S3CredentialLivetime
	if livetime == 0 {
		livetime = 1 * time.Hour
	}

	queryValues := url.Values{}
	queryValues.Set("Action", "AssumeRoleWithCertificate")
	queryValues.Set("Version", STSVersion)
	queryValues.Set("DurationSeconds", strconv.FormatUint(uint64(livetime.Seconds()), 10))
	if i.TokenRevokeType != "" {
		queryValues.Set("TokenRevokeType", i.TokenRevokeType)
	}
	endpointURL.RawQuery = queryValues.Encode()

	req, err := http.NewRequest(http.MethodPost, endpointURL.String(), nil)
	if err != nil {
		return Value{}, err
	}

	client := i.Client
	if client == nil {
		client = cc.Client
	}
	if client == nil {
		client = defaultCredContext.Client
	}

	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		return Value{}, fmt.Errorf("CredContext should contain an http.Transport value")
	}

	// Clone the HTTP transport (patch the TLS client certificate)
	trCopy := tr.Clone()
	trCopy.TLSClientConfig.Certificates = []tls.Certificate{i.Certificate}

	// Clone the HTTP client (patch the HTTP transport)
	clientCopy := *client
	clientCopy.Transport = trCopy

	resp, err := clientCopy.Do(req)
	if err != nil {
		return Value{}, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		buf, err := io.ReadAll(resp.Body)
		if err != nil {
			return Value{}, err
		}
		_, err = xmlDecodeAndBody(bytes.NewReader(buf), &errResp)
		if err != nil {
			var s3Err Error
			if _, err = xmlDecodeAndBody(bytes.NewReader(buf), &s3Err); err != nil {
				return Value{}, err
			}
			errResp.RequestID = s3Err.RequestID
			errResp.STSError.Code = s3Err.Code
			errResp.STSError.Message = s3Err.Message
		}
		return Value{}, errResp
	}

	const MaxSize = 10 * 1 << 20
	var body io.Reader = resp.Body
	if resp.ContentLength > 0 && resp.ContentLength < MaxSize {
		body = io.LimitReader(body, resp.ContentLength)
	} else {
		body = io.LimitReader(body, MaxSize)
	}

	var response assumeRoleWithCertificateResponse
	if err = xml.NewDecoder(body).Decode(&response); err != nil {
		return Value{}, err
	}
	i.SetExpiration(response.Result.Credentials.Expiration, DefaultExpiryWindow)
	return Value{
		AccessKeyID:     response.Result.Credentials.AccessKey,
		SecretAccessKey: response.Result.Credentials.SecretKey,
		SessionToken:    response.Result.Credentials.SessionToken,
		Expiration:      response.Result.Credentials.Expiration,
		SignerType:      SignatureDefault,
	}, nil
}

// Retrieve fetches a new set of S3 credentials from the configured STS API endpoint.
func (i *STSCertificateIdentity) Retrieve() (Value, error) {
	return i.RetrieveWithCredContext(defaultCredContext)
}

// Expiration returns the expiration time of the current S3 credentials.
func (i *STSCertificateIdentity) Expiration() time.Time { return i.expiration }

type assumeRoleWithCertificateResponse struct {
	XMLName xml.Name `xml:"https://sts.amazonaws.com/doc/2011-06-15/ AssumeRoleWithCertificateResponse" json:"-"`
	Result  struct {
		Credentials struct {
			AccessKey    string    `xml:"AccessKeyId" json:"accessKey,omitempty"`
			SecretKey    string    `xml:"SecretAccessKey" json:"secretKey,omitempty"`
			Expiration   time.Time `xml:"Expiration" json:"expiration,omitempty"`
			SessionToken string    `xml:"SessionToken" json:"sessionToken,omitempty"`
		} `xml:"Credentials" json:"credentials,omitempty"`
	} `xml:"AssumeRoleWithCertificateResult"`
	ResponseMetadata struct {
		RequestID string `xml:"RequestId,omitempty"`
	} `xml:"ResponseMetadata,omitempty"`
}
