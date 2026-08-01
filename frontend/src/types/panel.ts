// 面板路由标识。activePanel 是主区渲染的 12 种面板之一。
// 来源：WorkspaceView 否定链（characters/locations/storyarcs/timeline/
// reader/preferences/novel-settings/profile/git/style-samples 共 10 个）
// + BookshelfView 分支（novels）+ ContentPanel 默认（chapters）。
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
  | "style-samples";

// 侧栏可额外为 "search"（搜索面板覆盖在 SidePanel 内）。
export type SidebarPanelId = PanelId | "search";
