// 面板路由标识。activePanel 是主区渲染的 13 种面板之一。
// 来源：WorkspaceView 否定链排除的 10 个专门 View（characters/locations/
// storyarcs/timeline/reader/preferences/novel-settings/profile/git/style-samples）
// + BookshelfView 分支（novels）+ ContentPanel 默认（chapters、skills）。
export type PanelId =
  | "novels"
  | "chapters"
  | "characters"
  | "locations"
  | "storyarcs"
  | "timeline"
  | "reader"
  | "preferences"
  | "novel-settings"
  | "profile"
  | "git"
  | "skills"
  | "style-samples";

// 侧栏可额外为 "search"（搜索面板覆盖在 SidePanel 内）。
export type SidebarPanelId = PanelId | "search";
