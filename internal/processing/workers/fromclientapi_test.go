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

package workers_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/messages"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"code.superseriousbusiness.org/gotosocial/internal/stream"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/stretchr/testify/suite"
)

type FromClientAPITestSuite struct {
	WorkersTestSuite
}

func (suite *FromClientAPITestSuite) newStatus(
	ctx context.Context,
	state *state.State,
	account *gtsmodel.Account,
	visibility gtsmodel.Visibility,
	replyToStatus *gtsmodel.Status,
	boostOfStatus *gtsmodel.Status,
	mentionedAccounts []*gtsmodel.Account,
	createThread bool,
	tagIDs []string,
) *gtsmodel.Status {
	var (
		protocol = config.GetProtocol()
		host     = config.GetHost()
		statusID = id.NewULID()
	)

	// Make a new status from given account.
	newStatus := &gtsmodel.Status{
		ID:                  statusID,
		URI:                 protocol + "://" + host + "/users/" + account.Username + "/statuses/" + statusID,
		URL:                 protocol + "://" + host + "/@" + account.Username + "/statuses/" + statusID,
		Content:             "pee pee poo poo",
		TagIDs:              tagIDs,
		Local:               util.Ptr(true),
		AccountURI:          account.URI,
		AccountID:           account.ID,
		Visibility:          visibility,
		ActivityStreamsType: ap.ObjectNote,
		Federated:           util.Ptr(true),
	}

	if replyToStatus != nil {
		// Status is a reply.
		newStatus.InReplyToAccountID = replyToStatus.AccountID
		newStatus.InReplyToID = replyToStatus.ID
		newStatus.InReplyToURI = replyToStatus.URI
		newStatus.ThreadID = replyToStatus.ThreadID

		// Mention the replied-to account.
		mention := &gtsmodel.Mention{
			ID:               id.NewULID(),
			StatusID:         statusID,
			OriginAccountID:  account.ID,
			OriginAccountURI: account.URI,
			TargetAccountID:  replyToStatus.AccountID,
			IsNew:            true,
		}

		if err := state.DB.PutMention(ctx, mention); err != nil {
			suite.FailNow(err.Error())
		}
		newStatus.Mentions = []*gtsmodel.Mention{mention}
		newStatus.MentionIDs = []string{mention.ID}
	}

	if boostOfStatus != nil {
		// Status is a boost.
		newStatus.Content = ""
		newStatus.BoostOfAccountID = boostOfStatus.AccountID
		newStatus.BoostOfID = boostOfStatus.ID
		newStatus.Visibility = boostOfStatus.Visibility
	}

	for _, mentionedAccount := range mentionedAccounts {
		newMention := &gtsmodel.Mention{
			ID:               id.NewULID(),
			StatusID:         newStatus.ID,
			Status:           newStatus,
			OriginAccountID:  account.ID,
			OriginAccountURI: account.URI,
			OriginAccount:    account,
			TargetAccountID:  mentionedAccount.ID,
			TargetAccount:    mentionedAccount,
			Silent:           util.Ptr(false),
			IsNew:            true,
		}

		newStatus.Mentions = append(newStatus.Mentions, newMention)
		newStatus.MentionIDs = append(newStatus.MentionIDs, newMention.ID)

		if err := state.DB.PutMention(ctx, newMention); err != nil {
			suite.FailNow(err.Error())
		}
	}

	if createThread {
		newThread := &gtsmodel.Thread{
			ID: id.NewULID(),
		}

		newStatus.ThreadID = newThread.ID

		if err := state.DB.PutThread(ctx, newThread); err != nil {
			suite.FailNow(err.Error())
		}
	}

	// Put the status in the db, to mimic what would
	// have already happened earlier up the flow.
	if err := state.DB.PutStatus(ctx, newStatus); err != nil {
		suite.FailNow(err.Error())
	}

	return newStatus
}

func (suite *FromClientAPITestSuite) checkStreamed(
	str *stream.Stream,
	expectMessage bool,
	expectPayload string,
	expectEventType string,
) {

	// Set a 5s timeout on context.
	ctx := suite.T().Context()
	ctx, cncl := context.WithTimeout(ctx, time.Second*5)
	defer cncl()

	msg, ok := str.Recv(ctx)

	if expectMessage && !ok {
		suite.FailNow("expected a message but message was not received")
	}

	if !expectMessage && ok {
		suite.FailNow("expected no message but message was received")
	}

	if expectPayload != "" && msg.Payload != expectPayload {
		suite.FailNow("", "expected payload %s but payload was: %s", expectPayload, msg.Payload)
	}

	if expectEventType != "" && msg.Event != expectEventType {
		suite.FailNow("", "expected event type %s but event type was: %s", expectEventType, msg.Event)
	}
}

// checkWebPushed asserts that the target account got a single Web Push notification with a given type.
func (suite *FromClientAPITestSuite) checkWebPushed(
	sender *testrig.WebPushMockSender,
	accountID string,
	notificationType gtsmodel.NotificationType,
) {
	pushedNotifications := sender.Sent[accountID]
	if suite.Len(pushedNotifications, 1) {
		pushedNotification := pushedNotifications[0]
		suite.Equal(notificationType, pushedNotification.NotificationType)
	}
}

// checkNotWebPushed asserts that the target account got no Web Push notifications.
func (suite *FromClientAPITestSuite) checkNotWebPushed(
	sender *testrig.WebPushMockSender,
	accountID string,
) {
	pushedNotifications := sender.Sent[accountID]
	suite.Len(pushedNotifications, 0)
}

func (suite *FromClientAPITestSuite) statusJSON(
	ctx context.Context,
	typeConverter *typeutils.Converter,
	status *gtsmodel.Status,
	requestingAccount *gtsmodel.Account,
) string {
	apiStatus, err := typeConverter.StatusToAPIStatus(
		ctx,
		status,
		requestingAccount,
	)
	if err != nil {
		suite.FailNow(err.Error())
	}

	statusJSON, err := json.Marshal(apiStatus)
	if err != nil {
		suite.FailNow(err.Error())
	}

	return string(statusJSON)
}

func (suite *FromClientAPITestSuite) conversationJSON(
	ctx context.Context,
	typeConverter *typeutils.Converter,
	conversation *gtsmodel.Conversation,
	requestingAccount *gtsmodel.Account,
) string {
	apiConversation, err := typeConverter.ConversationToAPIConversation(
		ctx,
		conversation,
		requestingAccount,
	)
	if err != nil {
		suite.FailNow(err.Error())
	}

	conversationJSON, err := json.Marshal(apiConversation)
	if err != nil {
		suite.FailNow(err.Error())
	}

	return string(conversationJSON)
}

func (suite *FromClientAPITestSuite) TestProcessCreateStatusWithNotification() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_1"]
		testList         = suite.testLists["local_account_1_list_1"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			[]string{testList.ID},
		)
		publicStream = streams[stream.TimelinePublic]
		homeStream   = streams[stream.TimelineHome]
		listStream   = streams[stream.TimelineList+":"+testList.ID]
		notifStream  = streams[stream.TimelineNotifications]

		// Admin account posts a new top-level status.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			nil,
		)
	)

	// Update the follow from receiving account -> posting account so
	// that receiving account wants notifs when posting account posts.
	follow := new(gtsmodel.Follow)
	*follow = *suite.testFollows["local_account_1_admin_account"]

	follow.Notify = util.Ptr(true)
	if err := testStructs.State.DB.UpdateFollow(ctx, follow); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	statusJSON := suite.statusJSON(
		ctx,
		testStructs.TypeConverter,
		status,
		receivingAccount,
	)

	// Check message in public stream.
	suite.checkStreamed(
		publicStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Check message in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Check message in list stream.
	suite.checkStreamed(
		listStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Wait for a notification to appear for the status.
	var notif *gtsmodel.Notification
	if !testrig.WaitFor(func() bool {
		var err error
		notif, err = testStructs.State.DB.GetNotification(
			ctx,
			gtsmodel.NotificationStatus,
			receivingAccount.ID,
			postingAccount.ID,
			status.ID,
		)
		return err == nil
	}) {
		suite.FailNow("timed out waiting for new status notification")
	}

	apiNotif, err := testStructs.TypeConverter.NotificationToAPINotification(ctx, notif)
	if err != nil {
		suite.FailNow(err.Error())
	}

	notifJSON, err := json.Marshal(apiNotif)
	if err != nil {
		suite.FailNow(err.Error())
	}

	// Check message in notification stream.
	suite.checkStreamed(
		notifStream,
		true,
		string(notifJSON),
		stream.EventTypeNotification,
	)

	// Check for a Web Push status notification.
	suite.checkWebPushed(testStructs.WebPushSender, receivingAccount.ID, gtsmodel.NotificationStatus)
}

// Even with notifications on for a user, backfilling a status should not notify or timeline it.
func (suite *FromClientAPITestSuite) TestProcessCreateBackfilledStatusWithNotification() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_1"]
		testList         = suite.testLists["local_account_1_list_1"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			[]string{testList.ID},
		)
		publicStream = streams[stream.TimelinePublic]
		homeStream   = streams[stream.TimelineHome]
		listStream   = streams[stream.TimelineList+":"+testList.ID]
		notifStream  = streams[stream.TimelineNotifications]

		// Admin account posts a new top-level status.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			nil,
		)
	)

	// Update the follow from receiving account -> posting account so
	// that receiving account wants notifs when posting account posts.
	follow := new(gtsmodel.Follow)
	*follow = *suite.testFollows["local_account_1_admin_account"]

	follow.Notify = util.Ptr(true)
	if err := testStructs.State.DB.UpdateFollow(ctx, follow); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status as a backfill.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       &gtsmodel.BackfillStatus{Status: status},
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// There should be no message in public stream.
	suite.checkStreamed(
		publicStream,
		false,
		"",
		"",
	)

	// There should be no message in the home stream.
	suite.checkStreamed(
		homeStream,
		false,
		"",
		"",
	)

	// There should be no message in the list stream.
	suite.checkStreamed(
		listStream,
		false,
		"",
		"",
	)

	// No notification should appear for the status.
	if testrig.WaitFor(func() bool {
		var err error
		_, err = testStructs.State.DB.GetNotification(
			ctx,
			gtsmodel.NotificationStatus,
			receivingAccount.ID,
			postingAccount.ID,
			status.ID,
		)
		return err == nil
	}) {
		suite.FailNow("a status notification was created, but should not have been")
	}

	// There should be no message in the notification stream.
	suite.checkStreamed(
		notifStream,
		false,
		"",
		"",
	)

	// There should be no Web Push status notification.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

// Backfilled statuses should not federate when created.
func (suite *FromClientAPITestSuite) TestProcessCreateBackfilledStatusWithRemoteFollower() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["local_account_1"]
		receivingAccount = suite.testAccounts["remote_account_1"]

		// Local account posts a new top-level status.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			nil,
		)
	)

	// Follow the local account from the remote account.
	follow := &gtsmodel.Follow{
		ID:              "01JJHW9RW28SC1NEPZ0WBJQ4ZK",
		CreatedAt:       testrig.TimeMustParse("2022-05-14T13:21:09+02:00"),
		UpdatedAt:       testrig.TimeMustParse("2022-05-14T13:21:09+02:00"),
		AccountID:       receivingAccount.ID,
		TargetAccountID: postingAccount.ID,
		ShowReblogs:     util.Ptr(true),
		URI:             "http://fossbros-anonymous.io/users/foss_satan/follow/01JJHWEVC7F8W2JDW1136K431K",
		Notify:          util.Ptr(false),
	}

	if err := testStructs.State.DB.PutFollow(ctx, follow); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status as a backfill.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       &gtsmodel.BackfillStatus{Status: status},
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// No deliveries should be queued.
	suite.Zero(testStructs.State.Workers.Delivery.Queue.Len())
}

func (suite *FromClientAPITestSuite) TestProcessCreateStatusReply() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_1"]
		testList         = suite.testLists["local_account_1_list_1"]
		streams          = suite.openStreams(ctx, testStructs.Processor, receivingAccount, []string{testList.ID})
		publicStream     = streams[stream.TimelinePublic]
		homeStream       = streams[stream.TimelineHome]
		listStream       = streams[stream.TimelineList+":"+testList.ID]

		// Admin account posts a reply to turtle.
		// Since turtle is followed by zork, and
		// the default replies policy for this list
		// is to show replies to followed accounts,
		// post should also show in the list stream.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			suite.testStatuses["local_account_2_status_1"],
			nil,
			nil,
			false,
			nil,
		)
	)

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	statusJSON := suite.statusJSON(
		ctx,
		testStructs.TypeConverter,
		status,
		receivingAccount,
	)

	// Check message *not* in public stream.
	suite.checkStreamed(
		publicStream,
		false,
		"",
		"",
	)

	// Check message in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Check message in list stream.
	suite.checkStreamed(
		listStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

func (suite *FromClientAPITestSuite) TestProcessCreateStatusReplyMuted() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_1"]

		// Admin account posts a reply to zork.
		// Normally zork would get a notification
		// for this, but zork mutes this thread.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			suite.testStatuses["local_account_1_status_1"],
			nil,
			nil,
			false,
			nil,
		)
		threadMute = &gtsmodel.ThreadMute{
			ID:        "01HD3KRMBB1M85QRWHD912QWRE",
			ThreadID:  suite.testStatuses["local_account_1_status_1"].ThreadID,
			AccountID: receivingAccount.ID,
		}
	)

	// Store the thread mute before processing new status.
	if err := testStructs.State.DB.PutThreadMute(ctx, threadMute); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Ensure no notification received.
	notif, err := testStructs.State.DB.GetNotification(
		ctx,
		gtsmodel.NotificationMention,
		receivingAccount.ID,
		postingAccount.ID,
		status.ID,
	)

	suite.ErrorIs(err, db.ErrNoEntries)
	suite.Nil(notif)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

func (suite *FromClientAPITestSuite) TestProcessCreateStatusBoostMuted() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_1"]

		// Admin account boosts a status by zork.
		// Normally zork would get a notification
		// for this, but zork mutes this thread.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			suite.testStatuses["local_account_1_status_1"],
			nil,
			false,
			nil,
		)
		threadMute = &gtsmodel.ThreadMute{
			ID:        "01HD3KRMBB1M85QRWHD912QWRE",
			ThreadID:  suite.testStatuses["local_account_1_status_1"].ThreadID,
			AccountID: receivingAccount.ID,
		}
	)

	// Store the thread mute before processing new status.
	if err := testStructs.State.DB.PutThreadMute(ctx, threadMute); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ActivityAnnounce,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Ensure no notification received.
	notif, err := testStructs.State.DB.GetNotification(
		ctx,
		gtsmodel.NotificationReblog,
		receivingAccount.ID,
		postingAccount.ID,
		status.ID,
	)

	suite.ErrorIs(err, db.ErrNoEntries)
	suite.Nil(notif)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

func (suite *FromClientAPITestSuite) TestProcessCreateStatusListRepliesPolicyListOnlyOK() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	// We're modifying the test list so take a copy.
	testList := new(gtsmodel.List)
	*testList = *suite.testLists["local_account_1_list_1"]

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_1"]
		streams          = suite.openStreams(ctx, testStructs.Processor, receivingAccount, []string{testList.ID})
		publicStream     = streams[stream.TimelinePublic]
		homeStream       = streams[stream.TimelineHome]
		listStream       = streams[stream.TimelineList+":"+testList.ID]

		// Admin account posts a reply to turtle.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			suite.testStatuses["local_account_2_status_1"],
			nil,
			nil,
			false,
			nil,
		)
	)

	// Modify replies policy of test list to show replies
	// only to other accounts in the same list. Since turtle
	// and admin are in the same list, this means the reply
	// should be shown in the list.
	testList.RepliesPolicy = gtsmodel.RepliesPolicyList
	if err := testStructs.State.DB.UpdateList(ctx, testList, "replies_policy"); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	statusJSON := suite.statusJSON(
		ctx,
		testStructs.TypeConverter,
		status,
		receivingAccount,
	)

	// Check message *not* in public stream.
	suite.checkStreamed(
		publicStream,
		false,
		"",
		"",
	)

	// Check message in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Check message in list stream.
	suite.checkStreamed(
		listStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

func (suite *FromClientAPITestSuite) TestProcessCreateStatusListRepliesPolicyListOnlyNo() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	// We're modifying the test list so take a copy.
	testList := new(gtsmodel.List)
	*testList = *suite.testLists["local_account_1_list_1"]

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_1"]
		streams          = suite.openStreams(ctx, testStructs.Processor, receivingAccount, []string{testList.ID})
		publicStream     = streams[stream.TimelinePublic]
		homeStream       = streams[stream.TimelineHome]
		listStream       = streams[stream.TimelineList+":"+testList.ID]

		// Admin account posts a reply to turtle.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			suite.testStatuses["local_account_2_status_1"],
			nil,
			nil,
			false,
			nil,
		)
	)

	// Modify replies policy of test list to show replies
	// only to other accounts in the same list. We're
	// about to remove turtle from the same list as admin,
	// so the new post should not be streamed to the list.
	testList.RepliesPolicy = gtsmodel.RepliesPolicyList
	if err := testStructs.State.DB.UpdateList(ctx, testList, "replies_policy"); err != nil {
		suite.FailNow(err.Error())
	}

	// Remove turtle from the list.
	testEntry := suite.testListEntries["local_account_1_list_1_entry_1"]
	if err := testStructs.State.DB.DeleteListEntry(ctx, testEntry.ListID, testEntry.FollowID); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	statusJSON := suite.statusJSON(
		ctx,
		testStructs.TypeConverter,
		status,
		receivingAccount,
	)

	// Check message *not* in public stream.
	suite.checkStreamed(
		publicStream,
		false,
		"",
		"",
	)

	// Check message in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Check message NOT in list stream.
	suite.checkStreamed(
		listStream,
		false,
		"",
		"",
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

func (suite *FromClientAPITestSuite) TestProcessCreateStatusReplyListRepliesPolicyNone() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	// We're modifying the test list so take a copy.
	testList := new(gtsmodel.List)
	*testList = *suite.testLists["local_account_1_list_1"]

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_1"]
		streams          = suite.openStreams(ctx, testStructs.Processor, receivingAccount, []string{testList.ID})
		publicStream     = streams[stream.TimelinePublic]
		homeStream       = streams[stream.TimelineHome]
		listStream       = streams[stream.TimelineList+":"+testList.ID]

		// Admin account posts a reply to turtle.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			suite.testStatuses["local_account_2_status_1"],
			nil,
			nil,
			false,
			nil,
		)
	)

	// Modify replies policy of test list.
	// Since we're modifying the list to not
	// show any replies, the post should not
	// be streamed to the list.
	testList.RepliesPolicy = gtsmodel.RepliesPolicyNone
	if err := testStructs.State.DB.UpdateList(ctx, testList, "replies_policy"); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	statusJSON := suite.statusJSON(
		ctx,
		testStructs.TypeConverter,
		status,
		receivingAccount,
	)

	// Check message *not* in public stream.
	suite.checkStreamed(
		publicStream,
		false,
		"",
		"",
	)

	// Check message in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Check message NOT in list stream.
	suite.checkStreamed(
		listStream,
		false,
		"",
		"",
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

func (suite *FromClientAPITestSuite) TestProcessCreateStatusBoost() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_1"]
		testList         = suite.testLists["local_account_1_list_1"]
		streams          = suite.openStreams(ctx, testStructs.Processor, receivingAccount, []string{testList.ID})
		publicStream     = streams[stream.TimelinePublic]
		homeStream       = streams[stream.TimelineHome]
		listStream       = streams[stream.TimelineList+":"+testList.ID]

		// Admin account boosts a post by turtle.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			suite.testStatuses["local_account_2_status_1"],
			nil,
			false,
			nil,
		)
	)

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ActivityAnnounce,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	statusJSON := suite.statusJSON(
		ctx,
		testStructs.TypeConverter,
		status,
		receivingAccount,
	)

	// Check message *not* in public stream.
	suite.checkStreamed(
		publicStream,
		false,
		"",
		"",
	)

	// Check message in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Check message in list stream.
	suite.checkStreamed(
		listStream,
		true,
		statusJSON,
		stream.EventTypeUpdate,
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

func (suite *FromClientAPITestSuite) TestProcessCreateStatusBoostNoReblogs() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_1"]
		testList         = suite.testLists["local_account_1_list_1"]
		streams          = suite.openStreams(ctx, testStructs.Processor, receivingAccount, []string{testList.ID})
		publicStream     = streams[stream.TimelinePublic]
		homeStream       = streams[stream.TimelineHome]
		listStream       = streams[stream.TimelineList+":"+testList.ID]

		// Admin account boosts a post by turtle.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			suite.testStatuses["local_account_2_status_1"],
			nil,
			false,
			nil,
		)
	)

	// Update zork's follow of admin
	// to not show boosts in timeline.
	follow := new(gtsmodel.Follow)
	*follow = *suite.testFollows["local_account_1_admin_account"]
	follow.ShowReblogs = util.Ptr(false)
	if err := testStructs.State.DB.UpdateFollow(ctx, follow, "show_reblogs"); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ActivityAnnounce,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check message *not* in public stream.
	suite.checkStreamed(
		publicStream,
		false,
		"",
		"",
	)

	// Check message NOT in home stream.
	suite.checkStreamed(
		homeStream,
		false,
		"",
		"",
	)

	// Check message NOT in list stream.
	suite.checkStreamed(
		listStream,
		false,
		"",
		"",
	)
}

// A DM to a local user should create a conversation and accompanying notification.
func (suite *FromClientAPITestSuite) TestProcessCreateStatusWhichBeginsConversation() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["local_account_2"]
		receivingAccount = suite.testAccounts["local_account_1"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			nil,
		)
		homeStream   = streams[stream.TimelineHome]
		directStream = streams[stream.TimelineDirect]

		// turtle posts a new top-level DM mentioning zork.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityDirect,
			nil,
			nil,
			[]*gtsmodel.Account{receivingAccount},
			true,
			nil,
		)
	)

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Locate the conversation which should now exist for zork.
	conversation, err := testStructs.State.DB.GetConversationByThreadAndAccountIDs(
		ctx,
		status.ThreadID,
		receivingAccount.ID,
		[]string{postingAccount.ID},
	)
	if err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Check mention notification in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		"",
		stream.EventTypeNotification,
	)

	// Check conversation in direct stream.
	conversationJSON := suite.conversationJSON(
		ctx,
		testStructs.TypeConverter,
		conversation,
		receivingAccount,
	)
	suite.checkStreamed(
		directStream,
		true,
		conversationJSON,
		stream.EventTypeConversation,
	)

	// Check for a Web Push mention notification.
	suite.checkWebPushed(testStructs.WebPushSender, receivingAccount.ID, gtsmodel.NotificationMention)
}

// A public message to a local user should not result in a conversation notification.
func (suite *FromClientAPITestSuite) TestProcessCreateStatusWhichShouldNotCreateConversation() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["local_account_2"]
		receivingAccount = suite.testAccounts["local_account_1"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			nil,
		)
		homeStream   = streams[stream.TimelineHome]
		directStream = streams[stream.TimelineDirect]

		// turtle posts a new top-level public message mentioning zork.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			[]*gtsmodel.Account{receivingAccount},
			true,
			nil,
		)
	)

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Check mention notification in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		"",
		stream.EventTypeNotification,
	)

	// Check for absence of conversation notification in direct stream.
	suite.checkStreamed(
		directStream,
		false,
		"",
		"",
	)

	// Check for a Web Push mention notification.
	suite.checkWebPushed(testStructs.WebPushSender, receivingAccount.ID, gtsmodel.NotificationMention)
}

// A public status with a hashtag followed by a local user who does not otherwise follow the author
// should end up in the tag-following user's home timeline.
func (suite *FromClientAPITestSuite) TestProcessCreateStatusWithFollowedHashtag() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_2"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			nil,
		)
		homeStream = streams[stream.TimelineHome]
		testTag    = suite.testTags["welcome"]

		// postingAccount posts a new public status not mentioning anyone but using testTag.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			[]string{testTag.ID},
		)
	)

	// Check precondition: receivingAccount does not follow postingAccount.
	following, err := testStructs.State.DB.IsFollowing(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(following)

	// Check precondition: receivingAccount does not block postingAccount or vice versa.
	blocking, err := testStructs.State.DB.IsEitherBlocked(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(blocking)

	// Setup: receivingAccount follows testTag.
	if err := testStructs.State.DB.PutFollowedTag(ctx, receivingAccount.ID, testTag.ID); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

// A public status with a hashtag followed by a local user who does not otherwise follow the author
// should not end up in the tag-following user's home timeline
// if the user has the author blocked.
func (suite *FromClientAPITestSuite) TestProcessCreateStatusWithFollowedHashtagAndBlock() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["remote_account_1"]
		receivingAccount = suite.testAccounts["local_account_2"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			nil,
		)
		homeStream = streams[stream.TimelineHome]
		testTag    = suite.testTags["welcome"]

		// postingAccount posts a new public status not mentioning anyone but using testTag.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			[]string{testTag.ID},
		)
	)

	// Check precondition: receivingAccount does not follow postingAccount.
	following, err := testStructs.State.DB.IsFollowing(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(following)

	// Check precondition: postingAccount does not block receivingAccount.
	blocking, err := testStructs.State.DB.IsBlocked(ctx, postingAccount.ID, receivingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(blocking)

	// Check precondition: receivingAccount blocks postingAccount.
	blocking, err = testStructs.State.DB.IsBlocked(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.True(blocking)

	// Setup: receivingAccount follows testTag.
	if err := testStructs.State.DB.PutFollowedTag(ctx, receivingAccount.ID, testTag.ID); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in home stream.
	suite.checkStreamed(
		homeStream,
		false,
		"",
		"",
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

// A boost of a public status with a hashtag followed by a local user
// who does not otherwise follow the author or booster
// should end up in the tag-following user's home timeline as the original status.
func (suite *FromClientAPITestSuite) TestProcessCreateBoostWithFollowedHashtag() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["remote_account_2"]
		boostingAccount  = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_2"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			nil,
		)
		homeStream = streams[stream.TimelineHome]
		testTag    = suite.testTags["welcome"]

		// postingAccount posts a new public status not mentioning anyone but using testTag.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			[]string{testTag.ID},
		)

		// boostingAccount boosts that status.
		boost = suite.newStatus(
			ctx,
			testStructs.State,
			boostingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			status,
			nil,
			false,
			nil,
		)
	)

	// Check precondition: receivingAccount does not follow postingAccount.
	following, err := testStructs.State.DB.IsFollowing(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(following)

	// Check precondition: receivingAccount does not block postingAccount or vice versa.
	blocking, err := testStructs.State.DB.IsEitherBlocked(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(blocking)

	// Check precondition: receivingAccount does not follow boostingAccount.
	following, err = testStructs.State.DB.IsFollowing(ctx, receivingAccount.ID, boostingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(following)

	// Check precondition: receivingAccount does not block boostingAccount or vice versa.
	blocking, err = testStructs.State.DB.IsEitherBlocked(ctx, receivingAccount.ID, boostingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(blocking)

	// Setup: receivingAccount follows testTag.
	if err := testStructs.State.DB.PutFollowedTag(ctx, receivingAccount.ID, testTag.ID); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the boost.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ActivityAnnounce,
			APActivityType: ap.ActivityCreate,
			GTSModel:       boost,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

// A boost of a public status with a hashtag followed by a local user
// who does not otherwise follow the author or booster
// should not end up in the tag-following user's home timeline
// if the user has the author blocked.
func (suite *FromClientAPITestSuite) TestProcessCreateBoostWithFollowedHashtagAndBlock() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["remote_account_1"]
		boostingAccount  = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_2"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			nil,
		)
		homeStream = streams[stream.TimelineHome]
		testTag    = suite.testTags["welcome"]

		// postingAccount posts a new public status not mentioning anyone but using testTag.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			[]string{testTag.ID},
		)

		// boostingAccount boosts that status.
		boost = suite.newStatus(
			ctx,
			testStructs.State,
			boostingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			status,
			nil,
			false,
			nil,
		)
	)

	// Check precondition: receivingAccount does not follow postingAccount.
	following, err := testStructs.State.DB.IsFollowing(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(following)

	// Check precondition: postingAccount does not block receivingAccount.
	blocking, err := testStructs.State.DB.IsBlocked(ctx, postingAccount.ID, receivingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(blocking)

	// Check precondition: receivingAccount blocks postingAccount.
	blocking, err = testStructs.State.DB.IsBlocked(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.True(blocking)

	// Check precondition: receivingAccount does not follow boostingAccount.
	following, err = testStructs.State.DB.IsFollowing(ctx, receivingAccount.ID, boostingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(following)

	// Check precondition: receivingAccount does not block boostingAccount or vice versa.
	blocking, err = testStructs.State.DB.IsEitherBlocked(ctx, receivingAccount.ID, boostingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(blocking)

	// Setup: receivingAccount follows testTag.
	if err := testStructs.State.DB.PutFollowedTag(ctx, receivingAccount.ID, testTag.ID); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the boost.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ActivityAnnounce,
			APActivityType: ap.ActivityCreate,
			GTSModel:       boost,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in home stream.
	suite.checkStreamed(
		homeStream,
		false,
		"",
		"",
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

// A boost of a public status with a hashtag followed by a local user
// who does not otherwise follow the author or booster
// should not end up in the tag-following user's home timeline
// if the user has the booster blocked.
func (suite *FromClientAPITestSuite) TestProcessCreateBoostWithFollowedHashtagAndBlockedBoost() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		boostingAccount  = suite.testAccounts["remote_account_1"]
		receivingAccount = suite.testAccounts["local_account_2"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			nil,
		)
		homeStream = streams[stream.TimelineHome]
		testTag    = suite.testTags["welcome"]

		// postingAccount posts a new public status not mentioning anyone but using testTag.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			[]string{testTag.ID},
		)

		// boostingAccount boosts that status.
		boost = suite.newStatus(
			ctx,
			testStructs.State,
			boostingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			status,
			nil,
			false,
			nil,
		)
	)

	// Check precondition: receivingAccount does not follow postingAccount.
	following, err := testStructs.State.DB.IsFollowing(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(following)

	// Check precondition: receivingAccount does not block postingAccount or vice versa.
	blocking, err := testStructs.State.DB.IsEitherBlocked(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(blocking)

	// Check precondition: receivingAccount does not follow boostingAccount.
	following, err = testStructs.State.DB.IsFollowing(ctx, receivingAccount.ID, boostingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(following)

	// Check precondition: boostingAccount does not block receivingAccount.
	blocking, err = testStructs.State.DB.IsBlocked(ctx, boostingAccount.ID, receivingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(blocking)

	// Check precondition: receivingAccount blocks boostingAccount.
	blocking, err = testStructs.State.DB.IsBlocked(ctx, receivingAccount.ID, boostingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.True(blocking)

	// Setup: receivingAccount follows testTag.
	if err := testStructs.State.DB.PutFollowedTag(ctx, receivingAccount.ID, testTag.ID); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the boost.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ActivityAnnounce,
			APActivityType: ap.ActivityCreate,
			GTSModel:       boost,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in home stream.
	suite.checkStreamed(
		homeStream,
		false,
		"",
		"",
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

// A public status with a hashtag followed by a local user who follows the author and has them on an exclusive list
// should end up in the following user's timeline for that list, but not their home timeline.
func (suite *FromClientAPITestSuite) TestProcessCreateStatusWithAuthorOnExclusiveList() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["local_account_2"]
		receivingAccount = suite.testAccounts["local_account_1"]
		testList         = suite.testLists["local_account_1_list_1"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			[]string{testList.ID},
		)
		publicStream = streams[stream.TimelinePublic]
		homeStream   = streams[stream.TimelineHome]
		listStream   = streams[stream.TimelineList+":"+testList.ID]

		// postingAccount posts a new public status not mentioning anyone.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			nil,
		)
	)

	// Setup: make the list exclusive.
	// We modify the existing list rather than create a new one, so that there's only one list in play for this test.
	list := new(gtsmodel.List)
	*list = *testList
	list.Exclusive = util.Ptr(true)
	if err := testStructs.State.DB.UpdateList(ctx, list); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in public stream.
	suite.checkStreamed(
		publicStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Check status in list stream.
	suite.checkStreamed(
		listStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Check status not in home stream.
	suite.checkStreamed(
		homeStream,
		false,
		"",
		"",
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

// A public status with a hashtag followed by a local user who follows the author and has them on an exclusive list
// should end up in the following user's timeline for that list, but not their home timeline.
// This should happen regardless of whether the author is on any of the following user's *non*-exclusive lists.
func (suite *FromClientAPITestSuite) TestProcessCreateStatusWithAuthorOnExclusiveAndNonExclusiveLists() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx               = suite.T().Context()
		postingAccount    = suite.testAccounts["local_account_2"]
		receivingAccount  = suite.testAccounts["local_account_1"]
		testInclusiveList = suite.testLists["local_account_1_list_1"]
		testExclusiveList = &gtsmodel.List{
			ID:            id.NewULID(),
			Title:         "Cool Ass Posters From This Instance (exclusive)",
			AccountID:     receivingAccount.ID,
			RepliesPolicy: gtsmodel.RepliesPolicyFollowed,
			Exclusive:     util.Ptr(true),
		}
		testFollow               = suite.testFollows["local_account_1_local_account_2"]
		testExclusiveListEntries = []*gtsmodel.ListEntry{
			{
				ID:       id.NewULID(),
				ListID:   testExclusiveList.ID,
				FollowID: testFollow.ID,
			},
		}
		streams = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			[]string{
				testInclusiveList.ID,
				testExclusiveList.ID,
			},
		)
		publicStream        = streams[stream.TimelinePublic]
		homeStream          = streams[stream.TimelineHome]
		inclusiveListStream = streams[stream.TimelineList+":"+testInclusiveList.ID]
		exclusiveListStream = streams[stream.TimelineList+":"+testExclusiveList.ID]

		// postingAccount posts a new public status not mentioning anyone.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			nil,
		)
	)

	// Precondition: the pre-existing inclusive list should actually be inclusive.
	// This should be the case if we reset the DB correctly between tests in this file.
	{
		list, err := testStructs.State.DB.GetListByID(ctx, testInclusiveList.ID)
		if err != nil {
			suite.FailNow(err.Error())
		}
		if *list.Exclusive {
			suite.FailNowf(
				"test precondition failed: list %s should be inclusive, but isn't",
				testInclusiveList.ID,
			)
		}
	}

	// Setup: create the exclusive list and its list entry.
	if err := testStructs.State.DB.PutList(ctx, testExclusiveList); err != nil {
		suite.FailNow(err.Error())
	}
	if err := testStructs.State.DB.PutListEntries(ctx, testExclusiveListEntries); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in public stream.
	suite.checkStreamed(
		publicStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Check status in inclusive list stream.
	suite.checkStreamed(
		inclusiveListStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Check status in exclusive list stream.
	suite.checkStreamed(
		exclusiveListStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Check status not in home stream.
	suite.checkStreamed(
		homeStream,
		false,
		"",
		"",
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

// A public status with a hashtag followed by a local user who follows the author and has them on an exclusive list
// should end up in the following user's timeline for that list, but not their home timeline.
// When they have notifications on for that user, they should be notified.
func (suite *FromClientAPITestSuite) TestProcessCreateStatusWithAuthorOnExclusiveListAndNotificationsOn() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["local_account_2"]
		receivingAccount = suite.testAccounts["local_account_1"]
		testFollow       = suite.testFollows["local_account_1_local_account_2"]
		testList         = suite.testLists["local_account_1_list_1"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			[]string{testList.ID},
		)
		publicStream = streams[stream.TimelinePublic]
		homeStream   = streams[stream.TimelineHome]
		listStream   = streams[stream.TimelineList+":"+testList.ID]
		notifStream  = streams[stream.TimelineNotifications]

		// postingAccount posts a new public status not mentioning anyone.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			nil,
		)
	)

	// Setup: Update the follow from receiving account -> posting account so
	// that receiving account wants notifs when posting account posts.
	follow := new(gtsmodel.Follow)
	*follow = *testFollow
	follow.Notify = util.Ptr(true)
	if err := testStructs.State.DB.UpdateFollow(ctx, follow); err != nil {
		suite.FailNow(err.Error())
	}

	// Setup: make the list exclusive.
	list := new(gtsmodel.List)
	*list = *testList
	list.Exclusive = util.Ptr(true)
	if err := testStructs.State.DB.UpdateList(ctx, list); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the new status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityCreate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in public stream.
	suite.checkStreamed(
		publicStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Check status in list stream.
	suite.checkStreamed(
		listStream,
		true,
		"",
		stream.EventTypeUpdate,
	)

	// Wait for a notification to appear for the status.
	var notif *gtsmodel.Notification
	if !testrig.WaitFor(func() bool {
		var err error
		notif, err = testStructs.State.DB.GetNotification(
			ctx,
			gtsmodel.NotificationStatus,
			receivingAccount.ID,
			postingAccount.ID,
			status.ID,
		)
		return err == nil
	}) {
		suite.FailNow("timed out waiting for new status notification")
	}

	apiNotif, err := testStructs.TypeConverter.NotificationToAPINotification(ctx, notif)
	if err != nil {
		suite.FailNow(err.Error())
	}

	notifJSON, err := json.Marshal(apiNotif)
	if err != nil {
		suite.FailNow(err.Error())
	}

	// Check message in notification stream.
	suite.checkStreamed(
		notifStream,
		true,
		string(notifJSON),
		stream.EventTypeNotification,
	)

	// Check *notification* for status in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		string(notifJSON),
		stream.EventTypeNotification,
	)

	// Status itself should not be in home stream.
	suite.checkStreamed(
		homeStream,
		false,
		"",
		"",
	)

	// Check for a Web Push status notification.
	suite.checkWebPushed(testStructs.WebPushSender, receivingAccount.ID, gtsmodel.NotificationStatus)
}

// Updating a public status with a hashtag followed by a local user who does not otherwise follow the author
// should stream a status update to the tag-following user's home timeline.
func (suite *FromClientAPITestSuite) TestProcessUpdateStatusWithFollowedHashtag() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["admin_account"]
		receivingAccount = suite.testAccounts["local_account_2"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			nil,
		)
		homeStream = streams[stream.TimelineHome]
		testTag    = suite.testTags["welcome"]

		// postingAccount posts a new public status not mentioning anyone but using testTag.
		status = suite.newStatus(
			ctx,
			testStructs.State,
			postingAccount,
			gtsmodel.VisibilityPublic,
			nil,
			nil,
			nil,
			false,
			[]string{testTag.ID},
		)
	)

	// Check precondition: receivingAccount does not follow postingAccount.
	following, err := testStructs.State.DB.IsFollowing(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(following)

	// Check precondition: receivingAccount does not block postingAccount or vice versa.
	blocking, err := testStructs.State.DB.IsEitherBlocked(ctx, receivingAccount.ID, postingAccount.ID)
	if err != nil {
		suite.FailNow(err.Error())
	}
	suite.False(blocking)

	// Setup: receivingAccount follows testTag.
	if err := testStructs.State.DB.PutFollowedTag(ctx, receivingAccount.ID, testTag.ID); err != nil {
		suite.FailNow(err.Error())
	}

	// Update the status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityUpdate,
			GTSModel:       status,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Check status in home stream.
	suite.checkStreamed(
		homeStream,
		true,
		"",
		stream.EventTypeStatusUpdate,
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)
}

// Test that when someone edits a status that's been interacted with,
// the interacter gets a notification that the status has been edited.
func (suite *FromClientAPITestSuite) TestProcessUpdateStatusInteractedWith() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx              = suite.T().Context()
		postingAccount   = suite.testAccounts["local_account_1"]
		receivingAccount = suite.testAccounts["admin_account"]
		streams          = suite.openStreams(ctx,
			testStructs.Processor,
			receivingAccount,
			nil,
		)
		notifStream = streams[stream.TimelineNotifications]
	)

	// Copy the test status.
	//
	// This is one that the receiving account
	// has interacted with (by replying).
	testStatus := new(gtsmodel.Status)
	*testStatus = *suite.testStatuses["local_account_1_status_1"]

	// Create + store an edit.
	edit := &gtsmodel.StatusEdit{
		// Just set the ID + status ID, other
		// fields don't matter for this test.
		ID:       "01JTR74W15VS6A6MK15N5JVJ55",
		StatusID: testStatus.ID,
	}

	if err := testStructs.State.DB.PutStatusEdit(ctx, edit); err != nil {
		suite.FailNow(err.Error())
	}

	// Set edit on status as
	// it would be for real.
	testStatus.EditIDs = []string{edit.ID}
	testStatus.Edits = []*gtsmodel.StatusEdit{edit}

	// Update the status.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityUpdate,
			GTSModel:       testStatus,
			Origin:         postingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Wait for a notification to appear for the status.
	var notif *gtsmodel.Notification
	if !testrig.WaitFor(func() bool {
		var err error
		notif, err = testStructs.State.DB.GetNotification(
			ctx,
			gtsmodel.NotificationUpdate,
			receivingAccount.ID,
			postingAccount.ID,
			edit.ID,
		)
		return err == nil
	}) {
		suite.FailNow("timed out waiting for edited status notification")
	}

	apiNotif, err := testStructs.TypeConverter.NotificationToAPINotification(ctx, notif)
	if err != nil {
		suite.FailNow(err.Error())
	}

	notifJSON, err := json.Marshal(apiNotif)
	if err != nil {
		suite.FailNow(err.Error())
	}

	// Check notif in stream.
	suite.checkStreamed(
		notifStream,
		true,
		string(notifJSON),
		stream.EventTypeNotification,
	)
}

func (suite *FromClientAPITestSuite) TestProcessStatusDelete() {
	testStructs := testrig.SetupTestStructs(rMediaPath, rTemplatePath)
	defer testrig.TearDownTestStructs(testStructs)

	var (
		ctx                  = suite.T().Context()
		deletingAccount      = suite.testAccounts["local_account_1"]
		receivingAccount     = suite.testAccounts["local_account_2"]
		deletedStatus        = suite.testStatuses["local_account_1_status_1"]
		boostOfDeletedStatus = suite.testStatuses["admin_account_status_4"]
		streams              = suite.openStreams(ctx, testStructs.Processor, receivingAccount, nil)
		homeStream           = streams[stream.TimelineHome]
	)

	// Delete the status from the db first, to mimic what
	// would have already happened earlier up the flow
	if err := testStructs.State.DB.DeleteStatusByID(ctx, deletedStatus.ID); err != nil {
		suite.FailNow(err.Error())
	}

	// Process the status delete.
	if err := testStructs.Processor.Workers().ProcessFromClientAPI(
		ctx,
		&messages.FromClientAPI{
			APObjectType:   ap.ObjectNote,
			APActivityType: ap.ActivityDelete,
			GTSModel:       deletedStatus,
			Origin:         deletingAccount,
		},
	); err != nil {
		suite.FailNow(err.Error())
	}

	// Stream should have the delete
	// of admin's boost in it now.
	suite.checkStreamed(
		homeStream,
		true,
		boostOfDeletedStatus.ID,
		stream.EventTypeDelete,
	)

	// Stream should also have the delete
	// of the message itself in it.
	suite.checkStreamed(
		homeStream,
		true,
		deletedStatus.ID,
		stream.EventTypeDelete,
	)

	// Check for absence of Web Push notifications.
	suite.checkNotWebPushed(testStructs.WebPushSender, receivingAccount.ID)

	// Boost should no longer be in the database.
	if !testrig.WaitFor(func() bool {
		_, err := testStructs.State.DB.GetStatusByID(ctx, boostOfDeletedStatus.ID)
		return errors.Is(err, db.ErrNoEntries)
	}) {
		suite.FailNow("timed out waiting for status delete")
	}
}

func TestFromClientAPITestSuite(t *testing.T) {
	suite.Run(t, &FromClientAPITestSuite{})
}
