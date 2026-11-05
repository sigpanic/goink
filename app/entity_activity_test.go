package app

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/apperr"
)

func TestEntityActivityAfterSuccessfulCreation(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		name := "recorded"
		if unavailable {
			name = "statistics unavailable"
		}
		t.Run(name, func(t *testing.T) {
			a := setupTestApp(t)
			n := createTestNovel(t, a)
			ch := createTestChapter(t, a, n.ID)
			require.NoError(t, a.db.Where("1 = 1").Delete(&activity.DailyActivity{}).Error)
			if unavailable {
				require.NoError(t, a.db.Exec(`CREATE TRIGGER reject_activity BEFORE INSERT ON activity_daily
					BEGIN SELECT RAISE(ABORT, 'statistics unavailable'); END`).Error)
			}

			volumeName := "第一卷"
			v, err := a.PlaceVolume(PlaceVolumeInput{NovelID: n.ID, Name: &volumeName})
			require.NoError(t, err)
			_, err = a.PlaceVolume(PlaceVolumeInput{NovelID: n.ID, SourceVolumeID: &v.ID})
			require.NoError(t, err)
			_, err = a.CreateCharacterGroup(n.ID, CharacterGroupInput{Name: "主角团"})
			require.NoError(t, err)
			arc, err := a.CreateStoryArc(n.ID, CreateStoryArcInput{Name: "主线", ArcType: "main"})
			require.NoError(t, err)
			_, err = a.CreateArcNode(n.ID, CreateArcNodeInput{StoryArcID: arc.ID, Title: "转折", TargetReadingNumber: 1})
			require.NoError(t, err)
			_, err = a.CreateTimelineEntry(n.ID, CreateTimelineEntryInput{Category: "foreshadowing", Title: "伏笔", TargetReadingNumber: 1})
			require.NoError(t, err)
			_, err = a.CreateReaderPerspective(n.ID, CreateReaderPerspectiveInput{Type: "known", Content: "身份", PlantedChapterID: ch.ID})
			require.NoError(t, err)
			_, err = a.CreateNovelSetting(n.ID, CreateNovelSettingInput{Category: "世界观", Content: "设定"})
			require.NoError(t, err)
			_, err = a.CreatePreference(n.ID, CreatePreferenceInput{Category: "风格", Content: "偏好"})
			require.NoError(t, err)
			_, err = a.CreateStyleSample(CreateStyleSampleInput{NovelID: n.ID, Name: "素材", Content: "段落"})
			require.NoError(t, err)
			require.NoError(t, a.UpdateStoryArc(n.ID, arc.ID, UpdateStoryArcInput{Name: "更新主线", ArcType: "main", Status: "active"}))
			require.NoError(t, a.DeleteStoryArc(n.ID, arc.ID))
			_, err = a.CreateStoryArc(n.ID, CreateStoryArcInput{})
			require.Error(t, err)

			var rows []activity.DailyActivity
			require.NoError(t, a.db.Find(&rows).Error)
			if unavailable {
				require.Empty(t, rows)
				return
			}
			require.Len(t, rows, 1)
			require.Equal(t, activity.ActivityDelta{
				VolumesCreated: 1, CharacterGroupsCreated: 1, StoryArcsCreated: 1, ArcNodesCreated: 1,
				TimelineEntriesCreated: 1, ReaderPerspectivesCreated: 1, SettingsCreated: 1,
				PreferencesCreated: 1, StyleSamplesCreated: 1,
			}, rows[0].ActivityDelta)
		})
	}
}

func TestSkillCreationAndImageImportsActivity(t *testing.T) {
	a := setupTestApp(t)
	n := createTestNovel(t, a)
	empty := ""
	input := SaveContentInput{NovelID: n.ID, Path: "skills/test.md", Content: "---\nname: test\n---\n正文", ExpectedContent: &empty}
	require.NoError(t, a.SaveContent(input))
	previous := input.Content
	input.Content += "更新"
	input.ExpectedContent = &previous
	require.NoError(t, a.SaveContent(input))
	input.Path = "skills/without-expected.md"
	input.ExpectedContent = nil
	require.NoError(t, a.SaveContent(input))
	stale := "stale"
	input.ExpectedContent = &stale
	require.Error(t, a.SaveContent(input))
	input.ExpectedContent = nil
	input.Content = "---\nname: [invalid\n---\n正文"
	require.Error(t, a.SaveContent(input))

	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewGray(image.Rect(0, 0, 2, 2))))
	result := a.ImportChatImage(ChatImageInput{NovelID: n.ID, Name: "image.png", Data: base64.StdEncoding.EncodeToString(data.Bytes())})
	require.Equal(t, apperr.CodeOK, result.ErrCode)
	_, err := a.GetChatImage(n.ID, result.Data.ID)
	require.NoError(t, err)
	require.NoError(t, a.DeleteChatImage(n.ID, result.Data.ID))
	result = a.ImportChatImage(ChatImageInput{NovelID: n.ID, Data: "invalid"})
	require.NotEqual(t, apperr.CodeOK, result.ErrCode)

	var rows []activity.DailyActivity
	require.NoError(t, a.db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, activity.ActivityDelta{SkillsCreated: 1, ImagesImported: 1}, rows[0].ActivityDelta)
}
