package mscalendar

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/mattermost/mattermost-plugin-mscalendar/server/config"
	"github.com/mattermost/mattermost-plugin-mscalendar/server/mscalendar/mock_plugin_api"
	"github.com/mattermost/mattermost-plugin-mscalendar/server/remote"
	"github.com/mattermost/mattermost-plugin-mscalendar/server/remote/mock_remote"
	"github.com/mattermost/mattermost-plugin-mscalendar/server/store"
	"github.com/mattermost/mattermost-plugin-mscalendar/server/store/mock_store"
	"github.com/mattermost/mattermost-plugin-mscalendar/server/utils/bot/mock_bot"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestProcessAllDailySummary(t *testing.T) {
	for _, tc := range []struct {
		name          string
		err           string
		runAssertions func(deps *Dependencies, client remote.Client)
	}{
		{
			name: "Error fetching user admin",
			err:  "admin store error",
			runAssertions: func(deps *Dependencies, client remote.Client) {
				deps.IsAuthorizedAdmin = func(string) (bool, error) {
					return false, errors.New("admin store error")
				}
			},
		},
		{
			name: "User is not admin",
			err:  "Non-admin user attempting ProcessAllDailySummary bot_mm_id",
			runAssertions: func(deps *Dependencies, client remote.Client) {
				deps.IsAuthorizedAdmin = func(string) (bool, error) {
					return false, nil
				}
			},
		},
		{
			name: "Error fetching index",
			err:  "index store error",
			runAssertions: func(deps *Dependencies, client remote.Client) {
				s := deps.Store.(*mock_store.MockStore)
				s.EXPECT().LoadDailySummaryIndex().Return(store.DailySummaryIndex{}, errors.New("index store error"))
			},
		},
		{
			name: "No users",
			err:  "",
			runAssertions: func(deps *Dependencies, client remote.Client) {
				s := deps.Store.(*mock_store.MockStore)
				s.EXPECT().LoadDailySummaryIndex().Return(store.DailySummaryIndex{}, nil)
			},
		},
		{
			name: "Error fetching events",
			err:  "error fetching events",
			runAssertions: func(deps *Dependencies, client remote.Client) {
				s := deps.Store.(*mock_store.MockStore)
				s.EXPECT().LoadDailySummaryIndex().Return(store.DailySummaryIndex{
					&store.DailySummaryUserSettings{
						MattermostUserID: "user1_mm_id",
						RemoteID:         "user1_remote_id",
						Enable:           true,
						PostTime:         "9:00AM",
						Timezone:         "Eastern Standard Time",
						LastPostTime:     "",
					},
				}, nil).Times(1)

				token := &oauth2.Token{
					AccessToken: "bot_oauth_token",
				}
				s.EXPECT().LoadUser("bot_mm_id").Return(&store.User{
					MattermostUserID: "bot_mm_id",
					OAuth2Token:      token,
					Remote: &remote.User{
						ID:   "bot_remote_id",
						Mail: "bot_email@example.com",
					},
				}, nil).Times(1)

				mockPluginAPI := deps.PluginAPI.(*mock_plugin_api.MockPluginAPI)
				mockPluginAPI.EXPECT().GetMattermostUser("bot_mm_id").Return(&model.User{}, nil).Times(1)

				mockClient := client.(*mock_remote.MockClient)
				mockRemote := deps.Remote.(*mock_remote.MockRemote)
				mockRemote.EXPECT().MakeClient(context.Background(), token).Return(mockClient).Times(1)
				mockClient.EXPECT().GetSuperuserToken().Return("the token", nil).Times(1)
				mockRemote.EXPECT().MakeSuperuserClient(context.Background(), "the token").Return(mockClient).Times(1)

				mockClient.EXPECT().DoBatchViewCalendarRequests(gomock.Any()).Return([]*remote.ViewCalendarResponse{}, errors.New("error fetching events"))
			},
		},
		{
			name: "User receives their daily summary",
			err:  "",
			runAssertions: func(deps *Dependencies, client remote.Client) {
				s := deps.Store.(*mock_store.MockStore)
				s.EXPECT().LoadDailySummaryIndex().Return(store.DailySummaryIndex{
					&store.DailySummaryUserSettings{
						MattermostUserID: "user1_mm_id",
						RemoteID:         "user1_remote_id",
						Enable:           true,
						PostTime:         "9:00AM",
						Timezone:         "Eastern Standard Time",
						LastPostTime:     "",
					},
					&store.DailySummaryUserSettings{
						MattermostUserID: "user2_mm_id",
						RemoteID:         "user2_remote_id",
						Enable:           true,
						PostTime:         "6:00AM",
						Timezone:         "Pacific Standard Time",
						LastPostTime:     "",
					},
					&store.DailySummaryUserSettings{
						MattermostUserID: "user3_mm_id",
						RemoteID:         "user3_remote_id",
						Enable:           true,
						PostTime:         "10:00AM", // should not receive summary
						Timezone:         "Pacific Standard Time",
						LastPostTime:     "",
					},
				}, nil).Times(1)

				token := &oauth2.Token{
					AccessToken: "bot_oauth_token",
				}
				s.EXPECT().LoadUser("bot_mm_id").Return(&store.User{
					MattermostUserID: "bot_mm_id",
					OAuth2Token:      token,
					Remote: &remote.User{
						ID:   "bot_remote_id",
						Mail: "bot_email@example.com",
					},
				}, nil).Times(1)

				mockPluginAPI := deps.PluginAPI.(*mock_plugin_api.MockPluginAPI)
				mockPluginAPI.EXPECT().GetMattermostUser("bot_mm_id").Return(&model.User{}, nil).Times(1)

				mockClient := client.(*mock_remote.MockClient)
				loc, err := time.LoadLocation("MST")
				require.Nil(t, err)
				hour, minute := 10, 0 // Time is "10:00AM"
				moment := makeTime(hour, minute, loc)
				mockClient.EXPECT().DoBatchViewCalendarRequests(gomock.Any()).Return([]*remote.ViewCalendarResponse{
					{RemoteID: "user1_remote_id", Events: []*remote.Event{}},
					{RemoteID: "user2_remote_id", Events: []*remote.Event{
						{
							Subject: "The subject",
							Start:   remote.NewDateTime(moment, "Mountain Standard Time"),
							End:     remote.NewDateTime(moment.Add(2*time.Hour), "Mountain Standard Time"),
						},
					}},
				}, nil)
				mockRemote := deps.Remote.(*mock_remote.MockRemote)
				mockRemote.EXPECT().MakeClient(context.Background(), token).Return(mockClient).Times(1)
				mockClient.EXPECT().GetSuperuserToken().Return("the token", nil).Times(1)
				mockRemote.EXPECT().MakeSuperuserClient(context.Background(), "the token").Return(mockClient).Times(1)

				mockPoster := deps.Poster.(*mock_bot.MockPoster)
				gomock.InOrder(
					mockPoster.EXPECT().DM("user1_mm_id", "You have no upcoming events.").Return(nil).Times(1),
					mockPoster.EXPECT().DM("user2_mm_id", `Times are shown in Pacific Standard Time
Wednesday February 12
* 9:00AM - 11:00AM `+"`The subject`\n").Return(nil).Times(1),
				)

				s.EXPECT().ModifyDailySummaryIndex(gomock.Any()).Return(nil)

				mockLogger := deps.Logger.(*mock_bot.MockLogger)
				mockLogger.EXPECT().Infof("Processed daily summary for %d users", 2)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			s := mock_store.NewMockStore(ctrl)
			poster := mock_bot.NewMockPoster(ctrl)
			mockRemote := mock_remote.NewMockRemote(ctrl)
			mockClient := mock_remote.NewMockClient(ctrl)
			mockPluginAPI := mock_plugin_api.NewMockPluginAPI(ctrl)

			logger := mock_bot.NewMockLogger(ctrl)
			conf := &config.Config{BotUserID: "bot_mm_id"}
			env := Env{
				Config: conf,
				Dependencies: &Dependencies{
					Store:             s,
					Logger:            logger,
					Poster:            poster,
					Remote:            mockRemote,
					PluginAPI:         mockPluginAPI,
					IsAuthorizedAdmin: func(mattermostUserID string) (bool, error) { return true, nil },
				},
			}

			loc, err := time.LoadLocation("EST")
			require.Nil(t, err)
			hour, minute := 9, 0 // Time is "9:00AM"
			moment := makeTime(hour, minute, loc)

			tc.runAssertions(env.Dependencies, mockClient)
			mscalendar := New(env, "")
			err = mscalendar.ProcessAllDailySummary(moment)

			if tc.err != "" {
				require.Equal(t, tc.err, err.Error())
			} else {
				require.Nil(t, err)
			}

			require.NotNil(t, tc)
		})
	}
}

func TestShouldPostDailySummary(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		postTime    string
		timeZone    string
		shouldRun   bool
		shouldError bool
	}{
		{
			name:        "Disabled",
			enabled:     false,
			postTime:    "9:00AM",
			timeZone:    "Eastern Standard Time",
			shouldRun:   false,
			shouldError: false,
		},
		{
			name:        "Same timezone, wrong time",
			enabled:     true,
			postTime:    "8:00AM",
			timeZone:    "Eastern Standard Time",
			shouldRun:   false,
			shouldError: false,
		},
		{
			name:        "Same timezone, right time",
			enabled:     true,
			postTime:    "9:00AM",
			timeZone:    "Eastern Standard Time",
			shouldRun:   true,
			shouldError: false,
		},
		{
			name:        "Different timezone, wrong time",
			enabled:     true,
			postTime:    "9:00AM",
			timeZone:    "Mountain Standard Time",
			shouldRun:   false,
			shouldError: false,
		},
		{
			name:        "Different timezone, right time",
			enabled:     true,
			postTime:    "7:00AM",
			timeZone:    "Mountain Standard Time",
			shouldRun:   true,
			shouldError: false,
		},
		{
			name:        "Nepal timezone, wrong time",
			enabled:     true,
			postTime:    "7:00AM",
			timeZone:    "Nepal Standard Time",
			shouldRun:   false,
			shouldError: false,
		},
		{
			name:        "Nepal timezone, right time",
			enabled:     true,
			postTime:    "7:45PM",
			timeZone:    "Nepal Standard Time",
			shouldRun:   true,
			shouldError: false,
		},
		{
			enabled:     true,
			postTime:    "7:20FM", // Invalid time
			timeZone:    "Mountain Standard Time",
			shouldRun:   false,
			shouldError: true,
		},
		{
			enabled:     true,
			postTime:    "7:00AM",
			timeZone:    "Moon Time",
			shouldRun:   false,
			shouldError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loc, err := time.LoadLocation("EST")
			require.Nil(t, err)

			dsum := &store.DailySummaryUserSettings{
				Enable:   tc.enabled,
				PostTime: tc.postTime,
				Timezone: tc.timeZone,
			}

			hour, minute := 9, 0 // Time is "9:00AM"
			moment := makeTime(hour, minute, loc)

			shouldRun, err := shouldPostDailySummary(dsum, moment)
			require.Equal(t, tc.shouldRun, shouldRun)
			if tc.shouldError {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func makeTime(hour, minute int, loc *time.Location) time.Time {
	return time.Date(2020, 2, 12, hour, minute, 0, 0, loc)
}