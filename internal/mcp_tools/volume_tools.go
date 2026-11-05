package mcp_tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/volume"
)

func RegisterVolumeTools(r *Registry) {
	r.Register(&GetVolumesTool{})
	r.Register(&PlaceVolumeTool{})
	r.Register(&UpdateVolumeTool{})
}

type GetVolumesArgs struct{}

type GetVolumesTool struct{}

func (t *GetVolumesTool) Name() string { return "get_volumes" }
func (t *GetVolumesTool) Description() string {
	return "查询当前小说的全部分卷，按阅读顺序返回卷名、卷 ID 和卷纲路径。卷纲内容通过 read 读取；尚未编写的卷纲文件可能不存在。"
}
func (t *GetVolumesTool) Category() ToolCategory      { return CategoryNovelManagement }
func (t *GetVolumesTool) JSONSchema() json.RawMessage { return SchemaOf(GetVolumesArgs{}) }
func (t *GetVolumesTool) ExposeToLLM() bool           { return false }
func (t *GetVolumesTool) NewArgs() any                { return &GetVolumesArgs{} }

func (t *GetVolumesTool) Execute(ctx context.Context, _ any, tc ToolContext) (*ToolResult, error) {
	volumes, err := volume.NewStore(tc.DB, tc.LoggerOrDefault()).ListByNovel(ctx, nil, tc.NovelID)
	if err != nil {
		return nil, fmt.Errorf("查询小说分卷: %w", err)
	}
	var content strings.Builder
	if len(volumes) == 0 {
		content.WriteString("当前小说暂无分卷。")
	} else {
		fmt.Fprintf(&content, "共 %d 卷：\n", len(volumes))
		for i, v := range volumes {
			fmt.Fprintf(&content, "\n- 第%d卷 %s [volume_id:%d]；卷纲：`%s`", i+1, v.Name, v.ID, v.OutlineFilePath)
		}
	}
	return &ToolResult{Success: true, Data: map[string]any{"content": content.String()}}, nil
}

type PlaceVolumeArgs struct {
	SourceVolumeID *int64  `json:"source_volume_id,omitempty" jsonschema:"description=移动的卷 ID；创建时不传，与 name 二选一" validate:"required_without=Name,excluded_with=Name,omitempty,min=1"`
	Name           *string `json:"name,omitempty" jsonschema:"description=新卷名称；移动时不传，与 source_volume_id 二选一" validate:"required_without=SourceVolumeID,excluded_with=SourceVolumeID,omitempty"`
	BeforeVolumeID *int64  `json:"before_volume_id,omitempty" jsonschema:"description=放到此卷之前，不传或 null 则放到小说末尾" validate:"omitempty,min=1"`
}

type PlaceVolumeTool struct{}

func (t *PlaceVolumeTool) Name() string { return "place_volume" }
func (t *PlaceVolumeTool) Description() string {
	return "创建或移动当前小说的一个分卷。创建时传 name，移动时传 source_volume_id，二者互斥。before_volume_id 指定放到哪个已有卷之前，省略或 null 则放到末尾。创建不生成章节或卷纲文件；卷纲可用 edit 编写，重命名用 update_volume。"
}
func (t *PlaceVolumeTool) Category() ToolCategory      { return CategoryNovelManagement }
func (t *PlaceVolumeTool) JSONSchema() json.RawMessage { return SchemaOf(PlaceVolumeArgs{}) }
func (t *PlaceVolumeTool) ExposeToLLM() bool           { return false }
func (t *PlaceVolumeTool) NewArgs() any                { return &PlaceVolumeArgs{} }

func (t *PlaceVolumeTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*PlaceVolumeArgs)
	if a.Name != nil && strings.TrimSpace(*a.Name) == "" {
		return &ToolResult{Success: false, Error: "卷名不能为空"}, nil
	}
	v, err := volume.NewStore(tc.DB, tc.LoggerOrDefault()).Place(ctx, nil, volume.PlaceInput{
		NovelID: tc.NovelID, SourceVolumeID: a.SourceVolumeID,
		Name: a.Name, BeforeVolumeID: a.BeforeVolumeID,
	})
	if err != nil {
		if errors.Is(err, volume.ErrNameTaken) || errors.Is(err, volume.ErrNotFound) {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		return nil, fmt.Errorf("放置分卷失败: %w", err)
	}
	if a.SourceVolumeID == nil {
		activity.NewStore(tc.DB, tc.LoggerOrDefault()).AddActivity(ctx, activity.ActivityDelta{VolumesCreated: 1})
	}
	emitFileMetadataChanged(ctx, tc.NovelID, v.OutlineFilePath)
	return &ToolResult{Success: true, Data: map[string]any{"volume_id": v.ID}}, nil
}

type UpdateVolumeArgs struct {
	VolumeID int64  `json:"volume_id" jsonschema:"required,description=要重命名的卷 ID" validate:"required,min=1"`
	Name     string `json:"name" jsonschema:"required,description=新的卷名，同一小说内不可重复" validate:"required"`
}

type UpdateVolumeTool struct{}

func (t *UpdateVolumeTool) Name() string { return "update_volume" }
func (t *UpdateVolumeTool) Description() string {
	return "重命名当前小说的一个分卷，不改变卷顺序、章节归属或卷纲路径。调整卷顺序使用 place_volume。"
}
func (t *UpdateVolumeTool) Category() ToolCategory      { return CategoryNovelManagement }
func (t *UpdateVolumeTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateVolumeArgs{}) }
func (t *UpdateVolumeTool) ExposeToLLM() bool           { return false }
func (t *UpdateVolumeTool) NewArgs() any                { return &UpdateVolumeArgs{} }

func (t *UpdateVolumeTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateVolumeArgs)
	if strings.TrimSpace(a.Name) == "" {
		return &ToolResult{Success: false, Error: "卷名不能为空"}, nil
	}
	if err := volume.NewStore(tc.DB, tc.LoggerOrDefault()).Update(ctx, nil, tc.NovelID, a.VolumeID, a.Name); err != nil {
		if errors.Is(err, volume.ErrNameTaken) || errors.Is(err, volume.ErrNotFound) {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		return nil, fmt.Errorf("重命名分卷失败: %w", err)
	}
	emitFileMetadataChanged(ctx, tc.NovelID, git.VolumePath(a.VolumeID))
	return &ToolResult{Success: true, Data: map[string]any{"message": "卷名已更新"}}, nil
}
