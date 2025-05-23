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

package user_test

import (
	"code.superseriousbusiness.org/gotosocial/internal/admin"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/email"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/oauth"
	"code.superseriousbusiness.org/gotosocial/internal/processing/user"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/stretchr/testify/suite"
)

type UserStandardTestSuite struct {
	suite.Suite
	emailSender email.Sender
	db          db.DB
	state       state.State
	oauthServer oauth.Server

	testApps   map[string]*gtsmodel.Application
	testTokens map[string]*gtsmodel.Token
	testUsers  map[string]*gtsmodel.User
	sentEmails map[string]string

	user user.Processor
}

func (suite *UserStandardTestSuite) SetupTest() {
	suite.state.Caches.Init()

	testrig.InitTestConfig()
	testrig.InitTestLog()

	suite.db = testrig.NewTestDB(&suite.state)
	suite.state.DB = suite.db
	suite.state.AdminActions = admin.New(suite.state.DB, &suite.state.Workers)
	suite.oauthServer = testrig.NewTestOauthServer(&suite.state)

	suite.sentEmails = make(map[string]string)
	suite.emailSender = testrig.NewEmailSender("../../../web/template/", suite.sentEmails)
	suite.testApps = testrig.NewTestApplications()
	suite.testTokens = testrig.NewTestTokens()
	suite.testUsers = testrig.NewTestUsers()

	suite.user = user.New(&suite.state, typeutils.NewConverter(&suite.state), testrig.NewTestOauthServer(&suite.state), suite.emailSender)

	testrig.StandardDBSetup(suite.db, nil)
}

func (suite *UserStandardTestSuite) TearDownTest() {
	testrig.StandardDBTeardown(suite.db)
}
