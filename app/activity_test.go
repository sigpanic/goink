package app

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/config"
)

func TestSettingsSavePreservesActivityOrigin(t *testing.T) {
	a := setupTestApp(t)
	require.NotNil(t, a.activity)
	require.NotNil(t, a.settings.TrackingStartedAt)
	startedAt := *a.settings.TrackingStartedAt
	startDate := a.settings.TrackingStartDate
	require.NoError(t, a.SaveUserName("创作者"))
	saved, err := config.LoadSettings(a.db)
	require.NoError(t, err)
	require.NotNil(t, saved.TrackingStartedAt)
	require.True(t, startedAt.Equal(*saved.TrackingStartedAt))
	require.Equal(t, startDate, saved.TrackingStartDate)
	require.Equal(t, "创作者", saved.UserName)
	payload, err := json.Marshal(saved)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "tracking_")
	require.NotContains(t, string(payload), "Tracking")
}
