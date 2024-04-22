// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package mutes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apiutil "github.com/superseriousbusiness/gotosocial/internal/api/util"
	"github.com/superseriousbusiness/gotosocial/internal/gtserror"
	"github.com/superseriousbusiness/gotosocial/internal/oauth"
)

// MutesGETHandler swagger:operation GET /api/v1/mutes mutesGet
//
// Get an array of accounts that requesting account has muted.
//
// NOT IMPLEMENTED YET: Will currently always return an array of length 0.
//
// The next and previous queries can be parsed from the returned Link header.
// Example:
//
// ```
// <https://example.org/api/v1/mutes?limit=80&max_id=01FC0SKA48HNSVR6YKZCQGS2V8>; rel="next", <https://example.org/api/v1/mutes?limit=80&min_id=01FC0SKW5JK2Q4EVAV2B462YY0>; rel="prev"
// ````
//
//	---
//	tags:
//	- mutes
//
//	produces:
//	- application/json
//
//	parameters:
//	-
//		name: max_id
//		type: string
//		description: >-
//			Return only muted accounts *OLDER* than the given max ID.
//			The muted account with the specified ID will not be included in the response.
//			NOTE: the ID is of the internal mute, NOT any of the returned accounts.
//		in: query
//		required: false
//	-
//		name: since_id
//		type: string
//		description: >-
//			Return only muted accounts *NEWER* than the given since ID.
//			The muted account with the specified ID will not be included in the response.
//			NOTE: the ID is of the internal mute, NOT any of the returned accounts.
//		in: query
//	-
//		name: min_id
//		type: string
//		description: >-
//			Return only muted accounts *IMMEDIATELY NEWER* than the given min ID.
//			The muted account with the specified ID will not be included in the response.
//			NOTE: the ID is of the internal mute, NOT any of the returned accounts.
//		in: query
//		required: false
//	-
//		name: limit
//		type: integer
//		description: Number of muted accounts to return.
//		default: 40
//		minimum: 1
//		maximum: 80
//		in: query
//		required: false
//
//	security:
//	- OAuth2 Bearer:
//		- read:mutes
//
//	responses:
//		'200':
//			headers:
//				Link:
//					type: string
//					description: Links to the next and previous queries.
//			schema:
//				type: array
//				items:
//					"$ref": "#/definitions/account"
//		'400':
//			description: bad request
//		'401':
//			description: unauthorized
//		'404':
//			description: not found
//		'406':
//			description: not acceptable
//		'500':
//			description: internal server error
func (m *Module) MutesGETHandler(c *gin.Context) {
	if _, err := oauth.Authed(c, true, true, true, true); err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorUnauthorized(err, err.Error()), m.processor.InstanceGetV1)
		return
	}

	if _, err := apiutil.NegotiateAccept(c, apiutil.JSONAcceptHeaders...); err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorNotAcceptable(err, err.Error()), m.processor.InstanceGetV1)
		return
	}

	apiutil.Data(c, http.StatusOK, apiutil.AppJSON, apiutil.EmptyJSONArray)
}