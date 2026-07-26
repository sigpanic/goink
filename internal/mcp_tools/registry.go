package mcp_tools

// RegisterAllTools 注册全部 MCP 工具到注册表。
// 新增工具后在此方法中调用对应的 Register 函数。
func RegisterAllTools(r *Registry) {
	RegisterNovelTools(r)
	RegisterPreferenceTools(r)
	RegisterSettingTools(r)
	RegisterCharacterTools(r)
	RegisterReaderPerspectiveTools(r)
	RegisterLocationTools(r)
	RegisterTimelineTools(r)
	RegisterStoryArcTools(r)
	RegisterRWTools(r)
	RegisterMemoryTools(r)
	RegisterSubagentTools(r)
	RegisterDeleteTools(r)
	RegisterWebSearchTools(r)
	RegisterWebFetchTools(r)
}
