package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferences(t *testing.T) {
	a := setupTestApp(t)

	// Create a novel to attach preferences to.
	nv, err := a.CreateNovel(CreateNovelInput{
		Title: "Preference Novel",
		Genre: "drama",
	})
	require.NoError(t, err)

	// --- Create global preference ---
	globalPref, err := a.CreatePreference(nv.ID, CreatePreferenceInput{
		IsGlobal: true,
		Category: "style",
		Content:  "Use short sentences",
	})
	require.NoError(t, err)
	require.NotNil(t, globalPref)
	assert.True(t, globalPref.IsGlobal)
	assert.Equal(t, "style", globalPref.Category)
	assert.Equal(t, "Use short sentences", globalPref.Content)

	// --- Create novel-specific preference ---
	novelPref, err := a.CreatePreference(nv.ID, CreatePreferenceInput{
		IsGlobal: false,
		Category: "character",
		Content:  "Protagonist is quiet",
	})
	require.NoError(t, err)
	require.NotNil(t, novelPref)
	assert.False(t, novelPref.IsGlobal)
	assert.Equal(t, "character", novelPref.Category)
	assert.Equal(t, "Protagonist is quiet", novelPref.Content)

	// --- GetPreferences ---
	result, err := a.GetPreferences(nv.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Global, 1, "should have 1 global preference")
	assert.Equal(t, globalPref.ID, result.Global[0].ID)
	assert.Len(t, result.Novel, 1, "should have 1 novel preference")
	assert.Equal(t, novelPref.ID, result.Novel[0].ID)

	// --- Update preference ---
	updated, err := a.UpdatePreference(nv.ID, novelPref.ID, UpdatePreferenceInput{
		Category: "trait",
		Content:  "Protagonist is very quiet now",
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "trait", updated.Category)
	assert.Equal(t, "Protagonist is very quiet now", updated.Content)
	assert.False(t, updated.IsGlobal)

	// --- Delete preference ---
	err = a.DeletePreference(novelPref.ID)
	require.NoError(t, err)

	afterDel, err := a.GetPreferences(nv.ID)
	require.NoError(t, err)
	assert.Empty(t, afterDel.Novel, "novel preferences should be empty after deletion")
	assert.Len(t, afterDel.Global, 1, "global preference should still exist")
}
