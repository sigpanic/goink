export namespace app {
	
	export class ChatInput {
	    session_id: string;
	    novel_id: number;
	    message: string;
	    provider_name: string;
	    model_id: string;
	    reasoning_effort: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.novel_id = source["novel_id"];
	        this.message = source["message"];
	        this.provider_name = source["provider_name"];
	        this.model_id = source["model_id"];
	        this.reasoning_effort = source["reasoning_effort"];
	    }
	}
	export class ChatResult {
	    session_id: string;
	    turn_id: number;
	    final_text: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.turn_id = source["turn_id"];
	        this.final_text = source["final_text"];
	    }
	}
	export class CommitFileListResult {
	    commit?: git.CommitInfo;
	    files: git.FileEntry[];
	
	    static createFrom(source: any = {}) {
	        return new CommitFileListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commit = this.convertValues(source["commit"], git.CommitInfo);
	        this.files = this.convertValues(source["files"], git.FileEntry);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CompressInput {
	    session_id: string;
	    provider_name: string;
	    model_id: string;
	
	    static createFrom(source: any = {}) {
	        return new CompressInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.provider_name = source["provider_name"];
	        this.model_id = source["model_id"];
	    }
	}
	export class CompressResult {
	    turn_id: number;
	
	    static createFrom(source: any = {}) {
	        return new CompressResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.turn_id = source["turn_id"];
	    }
	}
	export class ComputeStyleStatsInput {
	    sample_ids: number[];
	
	    static createFrom(source: any = {}) {
	        return new ComputeStyleStatsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sample_ids = source["sample_ids"];
	    }
	}
	export class CreateArcNodeInput {
	    story_arc_id: number;
	    title: string;
	    description?: string;
	    target_chapter: number;
	
	    static createFrom(source: any = {}) {
	        return new CreateArcNodeInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.story_arc_id = source["story_arc_id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.target_chapter = source["target_chapter"];
	    }
	}
	export class CreateChapterInput {
	    novel_id: number;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateChapterInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	        this.title = source["title"];
	    }
	}
	export class CreateCharacterInput {
	    name: string;
	    description?: string;
	    personality?: string;
	    abilities?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateCharacterInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.personality = source["personality"];
	        this.abilities = source["abilities"];
	    }
	}
	export class CreateLocationInput {
	    name: string;
	    location_type?: string;
	    description?: string;
	    detail_json?: string;
	    parent_location_id?: number;
	    tags?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateLocationInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.location_type = source["location_type"];
	        this.description = source["description"];
	        this.detail_json = source["detail_json"];
	        this.parent_location_id = source["parent_location_id"];
	        this.tags = source["tags"];
	    }
	}
	export class CreateNovelInput {
	    title: string;
	    description?: string;
	    genre?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateNovelInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.description = source["description"];
	        this.genre = source["genre"];
	    }
	}
	export class CreateNovelSettingInput {
	    category: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateNovelSettingInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.content = source["content"];
	    }
	}
	export class CreatePreferenceInput {
	    is_global: boolean;
	    category: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new CreatePreferenceInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.is_global = source["is_global"];
	        this.category = source["category"];
	        this.content = source["content"];
	    }
	}
	export class CreateReaderPerspectiveInput {
	    type: string;
	    content: string;
	    planted_chapter: number;
	    related_truth?: string;
	    revealed_chapter?: number;
	
	    static createFrom(source: any = {}) {
	        return new CreateReaderPerspectiveInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.content = source["content"];
	        this.planted_chapter = source["planted_chapter"];
	        this.related_truth = source["related_truth"];
	        this.revealed_chapter = source["revealed_chapter"];
	    }
	}
	export class CreateStoryArcInput {
	    name: string;
	    arc_type: string;
	    description?: string;
	    importance?: number;
	
	    static createFrom(source: any = {}) {
	        return new CreateStoryArcInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.arc_type = source["arc_type"];
	        this.description = source["description"];
	        this.importance = source["importance"];
	    }
	}
	export class CreateStyleSampleInput {
	    novel_id: number;
	    is_global: boolean;
	    name: string;
	    content: string;
	    tags: string[];
	
	    static createFrom(source: any = {}) {
	        return new CreateStyleSampleInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	        this.is_global = source["is_global"];
	        this.name = source["name"];
	        this.content = source["content"];
	        this.tags = source["tags"];
	    }
	}
	export class CreateTimelineEntryInput {
	    category: string;
	    title: string;
	    content?: string;
	    detail_json?: string;
	    target_chapter: number;
	    importance?: number;
	    source_chapter?: number;
	    source?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateTimelineEntryInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.detail_json = source["detail_json"];
	        this.target_chapter = source["target_chapter"];
	        this.importance = source["importance"];
	        this.source_chapter = source["source_chapter"];
	        this.source = source["source"];
	    }
	}
	export class DeleteSkillInput {
	    novel_id: number;
	    name: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteSkillInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	        this.name = source["name"];
	        this.source = source["source"];
	    }
	}
	export class DeleteStyleSampleInput {
	    id: number;
	
	    static createFrom(source: any = {}) {
	        return new DeleteStyleSampleInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class ExtractStyleInput {
	    task_id: string;
	    sample_ids: number[];
	    provider_name: string;
	    model_id: string;
	    reasoning_effort: string;
	
	    static createFrom(source: any = {}) {
	        return new ExtractStyleInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.sample_ids = source["sample_ids"];
	        this.provider_name = source["provider_name"];
	        this.model_id = source["model_id"];
	        this.reasoning_effort = source["reasoning_effort"];
	    }
	}
	export class GetSessionsInput {
	    novel_id: number;
	    page: number;
	    size: number;
	    search: string;
	
	    static createFrom(source: any = {}) {
	        return new GetSessionsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	        this.page = source["page"];
	        this.size = source["size"];
	        this.search = source["search"];
	    }
	}
	export class ImportNovelInput {
	    file_path: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportNovelInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file_path = source["file_path"];
	    }
	}
	export class ImportWithLLMInput {
	    file_path: string;
	    provider_name: string;
	    model_id: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportWithLLMInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file_path = source["file_path"];
	        this.provider_name = source["provider_name"];
	        this.model_id = source["model_id"];
	    }
	}
	export class InstallRemoteSkillInput {
	    name: string;
	    target: string;
	    novel_id: number;
	
	    static createFrom(source: any = {}) {
	        return new InstallRemoteSkillInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.target = source["target"];
	        this.novel_id = source["novel_id"];
	    }
	}
	export class ListRemoteSkillsInput {
	    page: number;
	    size: number;
	    query: string;
	
	    static createFrom(source: any = {}) {
	        return new ListRemoteSkillsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.page = source["page"];
	        this.size = source["size"];
	        this.query = source["query"];
	    }
	}
	export class ListSkillsInput {
	    novel_id: number;
	
	    static createFrom(source: any = {}) {
	        return new ListSkillsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	    }
	}
	export class ListSlashCommandsInput {
	    novel_id: number;
	
	    static createFrom(source: any = {}) {
	        return new ListSlashCommandsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	    }
	}
	export class ListStyleSamplesInput {
	    novel_id: number;
	    page: number;
	    size: number;
	    search: string;
	
	    static createFrom(source: any = {}) {
	        return new ListStyleSamplesInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	        this.page = source["page"];
	        this.size = source["size"];
	        this.search = source["search"];
	    }
	}
	export class PreferenceResult {
	    global: preference.PreferenceItem[];
	    novel: preference.PreferenceItem[];
	    token_count: number;
	    over_budget: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PreferenceResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.global = this.convertValues(source["global"], preference.PreferenceItem);
	        this.novel = this.convertValues(source["novel"], preference.PreferenceItem);
	        this.token_count = source["token_count"];
	        this.over_budget = source["over_budget"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SaveContentInput {
	    novel_id: number;
	    path: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveContentInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	        this.path = source["path"];
	        this.content = source["content"];
	    }
	}
	export class SaveSettingsInput {
	
	
	    static createFrom(source: any = {}) {
	        return new SaveSettingsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class SessionDetail {
	    session_id: string;
	    novel_id: number;
	    title: string;
	    model: string;
	    reasoning_effort: string;
	    active_version: number;
	    last_turn_id: number;
	    usage?: number[];
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.novel_id = source["novel_id"];
	        this.title = source["title"];
	        this.model = source["model"];
	        this.reasoning_effort = source["reasoning_effort"];
	        this.active_version = source["active_version"];
	        this.last_turn_id = source["last_turn_id"];
	        this.usage = source["usage"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class SessionMeta {
	    session_id: string;
	    title: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.title = source["title"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class SetActiveNovelInput {
	    novel_id: number;
	
	    static createFrom(source: any = {}) {
	        return new SetActiveNovelInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	    }
	}
	export class SettingResult {
	    items: setting.SettingItem[];
	    token_count: number;
	    over_budget: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SettingResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], setting.SettingItem);
	        this.token_count = source["token_count"];
	        this.over_budget = source["over_budget"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SlashCommand {
	    name: string;
	    description: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new SlashCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.type = source["type"];
	    }
	}
	export class TestConnectionInput {
	    provider_name: string;
	    chat_url: string;
	    api_key: string;
	    model_id: string;
	
	    static createFrom(source: any = {}) {
	        return new TestConnectionInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider_name = source["provider_name"];
	        this.chat_url = source["chat_url"];
	        this.api_key = source["api_key"];
	        this.model_id = source["model_id"];
	    }
	}
	export class UpdateArcNodeInput {
	    title?: string;
	    description?: string;
	    target_chapter?: number;
	    actual_chapter?: number;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateArcNodeInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.description = source["description"];
	        this.target_chapter = source["target_chapter"];
	        this.actual_chapter = source["actual_chapter"];
	        this.status = source["status"];
	    }
	}
	export class UpdateChapterPlanInput {
	    scope: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateChapterPlanInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.content = source["content"];
	    }
	}
	export class UpdateCharacterInput {
	    name: string;
	    description: string;
	    abilities: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateCharacterInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.abilities = source["abilities"];
	    }
	}
	export class UpdateLocationInput {
	    name?: string;
	    location_type?: string;
	    description?: string;
	    detail_json?: string;
	    parent_location_id?: number;
	    tags?: string;
	    clear_parent?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UpdateLocationInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.location_type = source["location_type"];
	        this.description = source["description"];
	        this.detail_json = source["detail_json"];
	        this.parent_location_id = source["parent_location_id"];
	        this.tags = source["tags"];
	        this.clear_parent = source["clear_parent"];
	    }
	}
	export class UpdateNovelInput {
	    title?: string;
	    description?: string;
	    genre?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateNovelInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.description = source["description"];
	        this.genre = source["genre"];
	    }
	}
	export class UpdateNovelSettingInput {
	    category: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateNovelSettingInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.content = source["content"];
	    }
	}
	export class UpdatePreferenceInput {
	    category: string;
	    content: string;
	    is_global: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UpdatePreferenceInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.content = source["content"];
	        this.is_global = source["is_global"];
	    }
	}
	export class UpdateReaderPerspectiveInput {
	    type?: string;
	    content?: string;
	    planted_chapter?: number;
	    related_truth?: string;
	    revealed_chapter?: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateReaderPerspectiveInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.content = source["content"];
	        this.planted_chapter = source["planted_chapter"];
	        this.related_truth = source["related_truth"];
	        this.revealed_chapter = source["revealed_chapter"];
	    }
	}
	export class UpdateStoryArcInput {
	    name?: string;
	    description?: string;
	    arc_type?: string;
	    importance?: number;
	    status?: string;
	    reactivate_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateStoryArcInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.arc_type = source["arc_type"];
	        this.importance = source["importance"];
	        this.status = source["status"];
	        this.reactivate_at = source["reactivate_at"];
	    }
	}
	export class UpdateStyleSampleInput {
	    id: number;
	    name: string;
	    content: string;
	    tags: string[];
	    is_global: boolean;
	    novel_id: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateStyleSampleInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.content = source["content"];
	        this.tags = source["tags"];
	        this.is_global = source["is_global"];
	        this.novel_id = source["novel_id"];
	    }
	}
	export class UpdateTimelineEntryInput {
	    title: string;
	    content: string;
	    target_chapter: number;
	    importance: number;
	    status: string;
	    resolved_chapter: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateTimelineEntryInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.content = source["content"];
	        this.target_chapter = source["target_chapter"];
	        this.importance = source["importance"];
	        this.status = source["status"];
	        this.resolved_chapter = source["resolved_chapter"];
	    }
	}

}

export namespace apperr {
	
	export class Result__github_com_sigpanic_goink_internal_storage_PageResult_github_com_sigpanic_goink_internal_skill_remote_RemoteSkillMeta__ {
	    data?: storage.PageResult_github_com_sigpanic_goink_internal_skill_remote_RemoteSkillMeta_;
	    err_code: string;
	    err_msg?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result__github_com_sigpanic_goink_internal_storage_PageResult_github_com_sigpanic_goink_internal_skill_remote_RemoteSkillMeta__(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.data = this.convertValues(source["data"], storage.PageResult_github_com_sigpanic_goink_internal_skill_remote_RemoteSkillMeta_);
	        this.err_code = source["err_code"];
	        this.err_msg = source["err_msg"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Result_string_ {
	    data: string;
	    err_code: string;
	    err_msg?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result_string_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.data = source["data"];
	        this.err_code = source["err_code"];
	        this.err_msg = source["err_msg"];
	    }
	}
	export class Result_struct____ {
	    // Go type: struct {}
	    data: any;
	    err_code: string;
	    err_msg?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result_struct____(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.data = this.convertValues(source["data"], Object);
	        this.err_code = source["err_code"];
	        this.err_msg = source["err_msg"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace chapter {
	
	export class Chapter {
	    id: number;
	    novel_id: number;
	    chapter_number: number;
	    title: string;
	    summary: string;
	    word_count: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    file_path: string;
	
	    static createFrom(source: any = {}) {
	        return new Chapter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.chapter_number = source["chapter_number"];
	        this.title = source["title"];
	        this.summary = source["summary"];
	        this.word_count = source["word_count"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.file_path = source["file_path"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace character {
	
	export class Character {
	    id: number;
	    novel_id: number;
	    name: string;
	    description: string;
	    personality: string;
	    abilities: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Character(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.personality = source["personality"];
	        this.abilities = source["abilities"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CharacterRelation {
	    id: number;
	    novel_id: number;
	    source_character_id: number;
	    target_character_id: number;
	    relation_describe: string;
	    description: string;
	    chapter_number: number;
	    is_current: boolean;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new CharacterRelation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.source_character_id = source["source_character_id"];
	        this.target_character_id = source["target_character_id"];
	        this.relation_describe = source["relation_describe"];
	        this.description = source["description"];
	        this.chapter_number = source["chapter_number"];
	        this.is_current = source["is_current"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace config {
	
	export class AppSettings {
	    ID: number;
	    last_novel_id: number;
	    selected_model_key: string;
	    reasoning_effort: string;
	    approval_mode: string;
	    last_session_id: string;
	    user_name: string;
	    git_name: string;
	    git_email: string;
	    dismissed_version: string;
	    // Go type: time
	    last_update_check_at: any;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.last_novel_id = source["last_novel_id"];
	        this.selected_model_key = source["selected_model_key"];
	        this.reasoning_effort = source["reasoning_effort"];
	        this.approval_mode = source["approval_mode"];
	        this.last_session_id = source["last_session_id"];
	        this.user_name = source["user_name"];
	        this.git_name = source["git_name"];
	        this.git_email = source["git_email"];
	        this.dismissed_version = source["dismissed_version"];
	        this.last_update_check_at = this.convertValues(source["last_update_check_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace git {
	
	export class CommitInfo {
	    hash: string;
	    shortHash: string;
	    message: string;
	    // Go type: time
	    time: any;
	    authorName: string;
	    authorEmail: string;
	    filesChanged: number;
	    insertions: number;
	    deletions: number;
	
	    static createFrom(source: any = {}) {
	        return new CommitInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.shortHash = source["shortHash"];
	        this.message = source["message"];
	        this.time = this.convertValues(source["time"], null);
	        this.authorName = source["authorName"];
	        this.authorEmail = source["authorEmail"];
	        this.filesChanged = source["filesChanged"];
	        this.insertions = source["insertions"];
	        this.deletions = source["deletions"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FileDiff {
	    path: string;
	    changeType: string;
	    original: string;
	    modified: string;
	
	    static createFrom(source: any = {}) {
	        return new FileDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.changeType = source["changeType"];
	        this.original = source["original"];
	        this.modified = source["modified"];
	    }
	}
	export class FileEntry {
	    path: string;
	    changeType: string;
	    oldPath: string;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.changeType = source["changeType"];
	        this.oldPath = source["oldPath"];
	    }
	}

}

export namespace imp {
	
	export class SkippedChapter {
	    title: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new SkippedChapter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.reason = source["reason"];
	    }
	}
	export class ImportResult {
	    novel_id: number;
	    title: string;
	    chapter_count: number;
	    skipped_count: number;
	    skipped_chapters: SkippedChapter[];
	    needs_llm: boolean;
	    file_path?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	        this.title = source["title"];
	        this.chapter_count = source["chapter_count"];
	        this.skipped_count = source["skipped_count"];
	        this.skipped_chapters = this.convertValues(source["skipped_chapters"], SkippedChapter);
	        this.needs_llm = source["needs_llm"];
	        this.file_path = source["file_path"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace llm {
	
	export class AvailableModel {
	    Key: string;
	    ModelID: string;
	    ProviderName: string;
	    ModelName: string;
	    ContextWindow: number;
	    MaxOutputTokens: number;
	    SupportsThinking: boolean;
	    ReasoningLevels: string[];
	    SupportsVision: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AvailableModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Key = source["Key"];
	        this.ModelID = source["ModelID"];
	        this.ProviderName = source["ProviderName"];
	        this.ModelName = source["ModelName"];
	        this.ContextWindow = source["ContextWindow"];
	        this.MaxOutputTokens = source["MaxOutputTokens"];
	        this.SupportsThinking = source["SupportsThinking"];
	        this.ReasoningLevels = source["ReasoningLevels"];
	        this.SupportsVision = source["SupportsVision"];
	    }
	}
	export class ModelInfo {
	    id: string;
	    name: string;
	    context_window: number;
	    max_output_tokens: number;
	    supports_thinking: boolean;
	    reasoning_levels?: string[];
	    supports_vision: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.context_window = source["context_window"];
	        this.max_output_tokens = source["max_output_tokens"];
	        this.supports_thinking = source["supports_thinking"];
	        this.reasoning_levels = source["reasoning_levels"];
	        this.supports_vision = source["supports_vision"];
	    }
	}
	export class ProviderView {
	    key: string;
	    name: string;
	    chat_url: string;
	    api_key: string;
	    platform_url: string;
	    help_text: string;
	    temperature: number;
	    source: string;
	    builtin_models: ModelInfo[];
	    custom_models: ModelInfo[];
	
	    static createFrom(source: any = {}) {
	        return new ProviderView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.chat_url = source["chat_url"];
	        this.api_key = source["api_key"];
	        this.platform_url = source["platform_url"];
	        this.help_text = source["help_text"];
	        this.temperature = source["temperature"];
	        this.source = source["source"];
	        this.builtin_models = this.convertValues(source["builtin_models"], ModelInfo);
	        this.custom_models = this.convertValues(source["custom_models"], ModelInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LLMConfigView {
	    providers: ProviderView[];
	
	    static createFrom(source: any = {}) {
	        return new LLMConfigView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providers = this.convertValues(source["providers"], ProviderView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace location {
	
	export class Location {
	    id: number;
	    novel_id: number;
	    name: string;
	    location_type: string;
	    description: string;
	    detail_json: string;
	    parent_location_id?: number;
	    tags: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Location(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.name = source["name"];
	        this.location_type = source["location_type"];
	        this.description = source["description"];
	        this.detail_json = source["detail_json"];
	        this.parent_location_id = source["parent_location_id"];
	        this.tags = source["tags"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LocationRelation {
	    id: number;
	    novel_id: number;
	    location_a_id: number;
	    location_b_id: number;
	    relation_type: string;
	    description: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new LocationRelation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.location_a_id = source["location_a_id"];
	        this.location_b_id = source["location_b_id"];
	        this.relation_type = source["relation_type"];
	        this.description = source["description"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace novel {
	
	export class Novel {
	    id: number;
	    title: string;
	    genre: string;
	    description: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Novel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.genre = source["genre"];
	        this.description = source["description"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace pattern {
	
	export class BatchTrace {
	    index: number;
	    input_count: number;
	    output_count: number;
	    approx_tokens: number;
	
	    static createFrom(source: any = {}) {
	        return new BatchTrace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.input_count = source["input_count"];
	        this.output_count = source["output_count"];
	        this.approx_tokens = source["approx_tokens"];
	    }
	}
	export class BoundaryHint {
	    start_chapter: number;
	    end_chapter: number;
	    hint: string;
	
	    static createFrom(source: any = {}) {
	        return new BoundaryHint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start_chapter = source["start_chapter"];
	        this.end_chapter = source["end_chapter"];
	        this.hint = source["hint"];
	    }
	}
	export class ChapterSummaryItem {
	    chapter_number: number;
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new ChapterSummaryItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chapter_number = source["chapter_number"];
	        this.summary = source["summary"];
	    }
	}
	export class Chunk {
	    name: string;
	    start_chapter: number;
	    end_chapter: number;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new Chunk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.start_chapter = source["start_chapter"];
	        this.end_chapter = source["end_chapter"];
	        this.content = source["content"];
	    }
	}
	export class ChunkRoundTrace {
	    round: number;
	    input_count: number;
	    output_count: number;
	    token_count: number;
	    batches: BatchTrace[];
	    chunks: Chunk[];
	
	    static createFrom(source: any = {}) {
	        return new ChunkRoundTrace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.round = source["round"];
	        this.input_count = source["input_count"];
	        this.output_count = source["output_count"];
	        this.token_count = source["token_count"];
	        this.batches = this.convertValues(source["batches"], BatchTrace);
	        this.chunks = this.convertValues(source["chunks"], Chunk);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ExtractPatternInput {
	    task_id?: string;
	    novel_id: number;
	    provider_name: string;
	    model_id: string;
	    reasoning_effort: string;
	    chapter_ids?: number[];
	
	    static createFrom(source: any = {}) {
	        return new ExtractPatternInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.novel_id = source["novel_id"];
	        this.provider_name = source["provider_name"];
	        this.model_id = source["model_id"];
	        this.reasoning_effort = source["reasoning_effort"];
	        this.chapter_ids = source["chapter_ids"];
	    }
	}
	export class Trace {
	    task_id?: string;
	    novel_id: number;
	    chapter_count: number;
	    context_window: number;
	    batch_budget: number;
	    boundaries: BoundaryHint[];
	    summaries: ChapterSummaryItem[];
	    chunk_rounds: ChunkRoundTrace[];
	    final_tokens: number;
	
	    static createFrom(source: any = {}) {
	        return new Trace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.novel_id = source["novel_id"];
	        this.chapter_count = source["chapter_count"];
	        this.context_window = source["context_window"];
	        this.batch_budget = source["batch_budget"];
	        this.boundaries = this.convertValues(source["boundaries"], BoundaryHint);
	        this.summaries = this.convertValues(source["summaries"], ChapterSummaryItem);
	        this.chunk_rounds = this.convertValues(source["chunk_rounds"], ChunkRoundTrace);
	        this.final_tokens = source["final_tokens"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ExtractPatternResult {
	    task_id?: string;
	    name: string;
	    description: string;
	    raw_content: string;
	    file_path: string;
	    trace?: Trace;
	
	    static createFrom(source: any = {}) {
	        return new ExtractPatternResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.raw_content = source["raw_content"];
	        this.file_path = source["file_path"];
	        this.trace = this.convertValues(source["trace"], Trace);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace preference {
	
	export class PreferenceItem {
	    id: number;
	    novel_id: number;
	    is_global: boolean;
	    category: string;
	    content: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new PreferenceItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.is_global = source["is_global"];
	        this.category = source["category"];
	        this.content = source["content"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace reader {
	
	export class ReaderPerspective {
	    id: number;
	    novel_id: number;
	    type: string;
	    content: string;
	    related_truth: string;
	    planted_chapter: number;
	    revealed_chapter: number;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new ReaderPerspective(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.type = source["type"];
	        this.content = source["content"];
	        this.related_truth = source["related_truth"];
	        this.planted_chapter = source["planted_chapter"];
	        this.revealed_chapter = source["revealed_chapter"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace remote {
	
	export class RemoteSkillMeta {
	    name: string;
	    description: string;
	    category: string;
	    mode: string;
	    author: string;
	    version: number;
	    file: string;
	
	    static createFrom(source: any = {}) {
	        return new RemoteSkillMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.mode = source["mode"];
	        this.author = source["author"];
	        this.version = source["version"];
	        this.file = source["file"];
	    }
	}

}

export namespace search {
	
	export class Result {
	    type: string;
	    id: number;
	    title: string;
	    subtitle: string;
	    chapter_num: number;
	    file_path: string;
	    match_prefix: string;
	    match_hit: string;
	    match_suffix: string;
	    match_position: number;
	    match_len: number;
	    relevance: number;
	    panel_id: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.id = source["id"];
	        this.title = source["title"];
	        this.subtitle = source["subtitle"];
	        this.chapter_num = source["chapter_num"];
	        this.file_path = source["file_path"];
	        this.match_prefix = source["match_prefix"];
	        this.match_hit = source["match_hit"];
	        this.match_suffix = source["match_suffix"];
	        this.match_position = source["match_position"];
	        this.match_len = source["match_len"];
	        this.relevance = source["relevance"];
	        this.panel_id = source["panel_id"];
	    }
	}

}

export namespace session {
	
	export class Message {
	    id: number;
	    session_id: string;
	    turn_id: number;
	    role: string;
	    content: string;
	    thinking_content?: string;
	    token_count: number;
	    extra_metadata?: string;
	    version: number;
	    to_api: boolean;
	    to_frontend: boolean;
	    event_type?: string;
	    agent_type: string;
	    sub_task_id?: string;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session_id = source["session_id"];
	        this.turn_id = source["turn_id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.thinking_content = source["thinking_content"];
	        this.token_count = source["token_count"];
	        this.extra_metadata = source["extra_metadata"];
	        this.version = source["version"];
	        this.to_api = source["to_api"];
	        this.to_frontend = source["to_frontend"];
	        this.event_type = source["event_type"];
	        this.agent_type = source["agent_type"];
	        this.sub_task_id = source["sub_task_id"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace setting {
	
	export class SettingItem {
	    id: number;
	    novel_id: number;
	    category: string;
	    content: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new SettingItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.category = source["category"];
	        this.content = source["content"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace skill {
	
	export class SkillMeta {
	    name: string;
	    description: string;
	    category: string;
	    mode: string;
	    author: string;
	    version: number;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.mode = source["mode"];
	        this.author = source["author"];
	        this.version = source["version"];
	        this.source = source["source"];
	    }
	}

}

export namespace storage {
	
	export class PageResult_github_com_sigpanic_goink_app_SessionMeta_ {
	    items: app.SessionMeta[];
	    total: number;
	    page: number;
	    size: number;
	    total_pages: number;
	
	    static createFrom(source: any = {}) {
	        return new PageResult_github_com_sigpanic_goink_app_SessionMeta_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], app.SessionMeta);
	        this.total = source["total"];
	        this.page = source["page"];
	        this.size = source["size"];
	        this.total_pages = source["total_pages"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PageResult_github_com_sigpanic_goink_internal_skill_remote_RemoteSkillMeta_ {
	    items: remote.RemoteSkillMeta[];
	    total: number;
	    page: number;
	    size: number;
	    total_pages: number;
	
	    static createFrom(source: any = {}) {
	        return new PageResult_github_com_sigpanic_goink_internal_skill_remote_RemoteSkillMeta_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], remote.RemoteSkillMeta);
	        this.total = source["total"];
	        this.page = source["page"];
	        this.size = source["size"];
	        this.total_pages = source["total_pages"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PageResult_github_com_sigpanic_goink_internal_style_Sample_ {
	    items: style.Sample[];
	    total: number;
	    page: number;
	    size: number;
	    total_pages: number;
	
	    static createFrom(source: any = {}) {
	        return new PageResult_github_com_sigpanic_goink_internal_style_Sample_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], style.Sample);
	        this.total = source["total"];
	        this.page = source["page"];
	        this.size = source["size"];
	        this.total_pages = source["total_pages"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace storyarc {
	
	export class ArcNode {
	    id: number;
	    novel_id: number;
	    story_arc_id: number;
	    title: string;
	    description: string;
	    target_chapter: number;
	    actual_chapter: number;
	    status: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new ArcNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.story_arc_id = source["story_arc_id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.target_chapter = source["target_chapter"];
	        this.actual_chapter = source["actual_chapter"];
	        this.status = source["status"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StoryArc {
	    id: number;
	    novel_id: number;
	    name: string;
	    description: string;
	    arc_type: string;
	    importance: number;
	    status: string;
	    reactivate_at: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new StoryArc(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.arc_type = source["arc_type"];
	        this.importance = source["importance"];
	        this.status = source["status"];
	        this.reactivate_at = source["reactivate_at"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace style {
	
	export class ExtractResult {
	    name: string;
	    description: string;
	    raw_content: string;
	    file_path: string;
	
	    static createFrom(source: any = {}) {
	        return new ExtractResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.raw_content = source["raw_content"];
	        this.file_path = source["file_path"];
	    }
	}
	export class Sample {
	    id: number;
	    novel_id: number;
	    is_global: boolean;
	    name: string;
	    content: string;
	    preview: string;
	    tags: string[];
	    word_count: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Sample(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.is_global = source["is_global"];
	        this.name = source["name"];
	        this.content = source["content"];
	        this.preview = source["preview"];
	        this.tags = source["tags"];
	        this.word_count = source["word_count"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Stats {
	    total_chars: number;
	    total_words: number;
	    sentence_count: number;
	    short_sent_pct: number;
	    mid_sent_pct: number;
	    long_sent_pct: number;
	    avg_sent_len: number;
	    sent_len_std_dev: number;
	    comma_density: number;
	    period_density: number;
	    exclaim_density: number;
	    question_density: number;
	    quote_density: number;
	    paragraph_count: number;
	    avg_para_len: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_chars = source["total_chars"];
	        this.total_words = source["total_words"];
	        this.sentence_count = source["sentence_count"];
	        this.short_sent_pct = source["short_sent_pct"];
	        this.mid_sent_pct = source["mid_sent_pct"];
	        this.long_sent_pct = source["long_sent_pct"];
	        this.avg_sent_len = source["avg_sent_len"];
	        this.sent_len_std_dev = source["sent_len_std_dev"];
	        this.comma_density = source["comma_density"];
	        this.period_density = source["period_density"];
	        this.exclaim_density = source["exclaim_density"];
	        this.question_density = source["question_density"];
	        this.quote_density = source["quote_density"];
	        this.paragraph_count = source["paragraph_count"];
	        this.avg_para_len = source["avg_para_len"];
	    }
	}

}

export namespace timeline {
	
	export class ChapterPlan {
	    novel_id: number;
	    scope: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new ChapterPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.novel_id = source["novel_id"];
	        this.scope = source["scope"];
	        this.content = source["content"];
	    }
	}
	export class TimelineEntry {
	    id: number;
	    novel_id: number;
	    category: string;
	    status: string;
	    title: string;
	    content: string;
	    detail_json: string;
	    target_chapter: number;
	    importance: number;
	    source_chapter: number;
	    source: string;
	    resolved_chapter: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new TimelineEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.novel_id = source["novel_id"];
	        this.category = source["category"];
	        this.status = source["status"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.detail_json = source["detail_json"];
	        this.target_chapter = source["target_chapter"];
	        this.importance = source["importance"];
	        this.source_chapter = source["source_chapter"];
	        this.source = source["source"];
	        this.resolved_chapter = source["resolved_chapter"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace update {
	
	export class ReleaseInfo {
	    tag_name: string;
	    name: string;
	    body: string;
	    html_url: string;
	    published_at: string;
	
	    static createFrom(source: any = {}) {
	        return new ReleaseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tag_name = source["tag_name"];
	        this.name = source["name"];
	        this.body = source["body"];
	        this.html_url = source["html_url"];
	        this.published_at = source["published_at"];
	    }
	}
	export class CheckResult {
	    currentVersion: string;
	    latest: ReleaseInfo;
	    hasUpdate: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.latest = this.convertValues(source["latest"], ReleaseInfo);
	        this.hasUpdate = source["hasUpdate"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace writing {
	
	export class DailyActivity {
	    date: string;
	    words: number;
	
	    static createFrom(source: any = {}) {
	        return new DailyActivity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.words = source["words"];
	    }
	}
	export class WritingStats {
	    total_words: number;
	    total_days_active: number;
	    current_streak: number;
	    longest_streak: number;
	    total_novels: number;
	    total_chapters: number;
	
	    static createFrom(source: any = {}) {
	        return new WritingStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_words = source["total_words"];
	        this.total_days_active = source["total_days_active"];
	        this.current_streak = source["current_streak"];
	        this.longest_streak = source["longest_streak"];
	        this.total_novels = source["total_novels"];
	        this.total_chapters = source["total_chapters"];
	    }
	}

}

