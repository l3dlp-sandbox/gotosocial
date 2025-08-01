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

package processing_test

import (
	"context"

	"code.superseriousbusiness.org/gotosocial/internal/admin"
	apiutil "code.superseriousbusiness.org/gotosocial/internal/api/util"
	"code.superseriousbusiness.org/gotosocial/internal/cleaner"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/email"
	"code.superseriousbusiness.org/gotosocial/internal/federation"
	"code.superseriousbusiness.org/gotosocial/internal/filter/interaction"
	"code.superseriousbusiness.org/gotosocial/internal/filter/mutes"
	"code.superseriousbusiness.org/gotosocial/internal/filter/status"
	"code.superseriousbusiness.org/gotosocial/internal/filter/visibility"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/media"
	"code.superseriousbusiness.org/gotosocial/internal/oauth"
	"code.superseriousbusiness.org/gotosocial/internal/processing"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/storage"
	"code.superseriousbusiness.org/gotosocial/internal/stream"
	"code.superseriousbusiness.org/gotosocial/internal/subscriptions"
	"code.superseriousbusiness.org/gotosocial/internal/transport"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/stretchr/testify/suite"
)

type ProcessingStandardTestSuite struct {
	// standard suite interfaces
	suite.Suite
	db                  db.DB
	storage             *storage.Driver
	state               state.State
	mediaManager        *media.Manager
	typeconverter       *typeutils.Converter
	httpClient          *testrig.MockHTTPClient
	transportController transport.Controller
	federator           *federation.Federator
	oauthServer         oauth.Server
	emailSender         email.Sender

	// standard suite models
	testTokens       map[string]*gtsmodel.Token
	testApplications map[string]*gtsmodel.Application
	testUsers        map[string]*gtsmodel.User
	testAccounts     map[string]*gtsmodel.Account
	testFollows      map[string]*gtsmodel.Follow
	testAttachments  map[string]*gtsmodel.MediaAttachment
	testStatuses     map[string]*gtsmodel.Status
	testTags         map[string]*gtsmodel.Tag
	testMentions     map[string]*gtsmodel.Mention
	testAutheds      map[string]*apiutil.Auth
	testBlocks       map[string]*gtsmodel.Block
	testActivities   map[string]testrig.ActivityWithSignature
	testLists        map[string]*gtsmodel.List

	processor *processing.Processor
}

func (suite *ProcessingStandardTestSuite) SetupSuite() {
	suite.testTokens = testrig.NewTestTokens()
	suite.testApplications = testrig.NewTestApplications()
	suite.testUsers = testrig.NewTestUsers()
	suite.testAccounts = testrig.NewTestAccounts()
	suite.testFollows = testrig.NewTestFollows()
	suite.testAttachments = testrig.NewTestAttachments()
	suite.testStatuses = testrig.NewTestStatuses()
	suite.testTags = testrig.NewTestTags()
	suite.testMentions = testrig.NewTestMentions()
	suite.testAutheds = map[string]*apiutil.Auth{
		"local_account_1": {
			Application: suite.testApplications["local_account_1"],
			User:        suite.testUsers["local_account_1"],
			Account:     suite.testAccounts["local_account_1"],
		},
	}
	suite.testBlocks = testrig.NewTestBlocks()
	suite.testLists = testrig.NewTestLists()
}

func (suite *ProcessingStandardTestSuite) SetupTest() {
	suite.state.Caches.Init()

	testrig.InitTestConfig()
	testrig.InitTestLog()

	suite.db = testrig.NewTestDB(&suite.state)
	suite.state.DB = suite.db
	suite.state.AdminActions = admin.New(suite.state.DB, &suite.state.Workers)
	suite.testActivities = testrig.NewTestActivities(suite.testAccounts)
	suite.storage = testrig.NewInMemoryStorage()
	suite.state.Storage = suite.storage
	suite.typeconverter = typeutils.NewConverter(&suite.state)

	suite.httpClient = testrig.NewMockHTTPClient(nil, "../../testrig/media")
	suite.httpClient.TestRemotePeople = testrig.NewTestFediPeople()
	suite.httpClient.TestRemoteStatuses = testrig.NewTestFediStatuses()

	suite.transportController = testrig.NewTestTransportController(&suite.state, suite.httpClient)
	suite.mediaManager = testrig.NewTestMediaManager(&suite.state)
	suite.federator = testrig.NewTestFederator(&suite.state, suite.transportController, suite.mediaManager)
	suite.oauthServer = testrig.NewTestOauthServer(&suite.state)
	suite.emailSender = testrig.NewEmailSender("../../web/template/", nil)

	suite.processor = processing.NewProcessor(
		cleaner.New(&suite.state),
		subscriptions.New(&suite.state, suite.transportController, suite.typeconverter),
		suite.typeconverter,
		suite.federator,
		suite.oauthServer,
		suite.mediaManager,
		&suite.state,
		suite.emailSender,
		testrig.NewNoopWebPushSender(),
		visibility.NewFilter(&suite.state),
		mutes.NewFilter(&suite.state),
		interaction.NewFilter(&suite.state),
		status.NewFilter(&suite.state),
	)
	testrig.StartWorkers(&suite.state, suite.processor.Workers())

	testrig.StandardDBSetup(suite.db, suite.testAccounts)
	testrig.StandardStorageSetup(suite.storage, "../../testrig/media")
}

func (suite *ProcessingStandardTestSuite) TearDownTest() {
	testrig.StandardDBTeardown(suite.db)
	testrig.StandardStorageTeardown(suite.storage)
	testrig.StopWorkers(&suite.state)
}

func (suite *ProcessingStandardTestSuite) openStreams(ctx context.Context, account *gtsmodel.Account, listIDs []string) map[string]*stream.Stream {
	streams := make(map[string]*stream.Stream)

	for _, streamType := range []string{
		stream.TimelineHome,
		stream.TimelinePublic,
		stream.TimelineNotifications,
	} {
		stream, err := suite.processor.Stream().Open(ctx, account, streamType)
		if err != nil {
			suite.FailNow(err.Error())
		}

		streams[streamType] = stream
	}

	for _, listID := range listIDs {
		streamType := stream.TimelineList + ":" + listID

		stream, err := suite.processor.Stream().Open(ctx, account, streamType)
		if err != nil {
			suite.FailNow(err.Error())
		}

		streams[streamType] = stream
	}

	return streams
}
