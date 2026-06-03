export namespace handler {
	
	export class Attachment {
	    name: string;
	    mime_type: string;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.mime_type = source["mime_type"];
	        this.data = source["data"];
	    }
	}
	export class BackupFile {
	    name: string;
	    size: number;
	    mod_time: string;
	
	    static createFrom(source: any = {}) {
	        return new BackupFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.mod_time = source["mod_time"];
	    }
	}
	export class SendMessageRequest {
	    conversation_id: string;
	    content: string;
	    provider: string;
	    model: string;
	    agent_id: string;
	    mcp_server_ids: string[];
	    skill_ids: string[];
	    web_search: boolean;
	    ignore_context: boolean;
	    context_cutoff_id: string;
	    attachments: Attachment[];
	    mode: string;
	    knowledge_base_id: string;
	    regenerate_group_id: string;
	
	    static createFrom(source: any = {}) {
	        return new SendMessageRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.content = source["content"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.agent_id = source["agent_id"];
	        this.mcp_server_ids = source["mcp_server_ids"];
	        this.skill_ids = source["skill_ids"];
	        this.web_search = source["web_search"];
	        this.ignore_context = source["ignore_context"];
	        this.context_cutoff_id = source["context_cutoff_id"];
	        this.attachments = this.convertValues(source["attachments"], Attachment);
	        this.mode = source["mode"];
	        this.knowledge_base_id = source["knowledge_base_id"];
	        this.regenerate_group_id = source["regenerate_group_id"];
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
	export class WebDAVConfig {
	    url: string;
	    username: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new WebDAVConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.username = source["username"];
	        this.path = source["path"];
	    }
	}

}

export namespace kb {
	
	export class KBDocument {
	    id: string;
	    name: string;
	    mime_type: string;
	    size: number;
	    chunk_count: number;
	    status: string;
	    error: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new KBDocument(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.mime_type = source["mime_type"];
	        this.size = source["size"];
	        this.chunk_count = source["chunk_count"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.created_at = source["created_at"];
	    }
	}

}

export namespace storage {
	
	export class Agent {
	    id: string;
	    name: string;
	    icon: string;
	    description: string;
	    system_prompt: string;
	    sort_order: number;
	    builtin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Agent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.icon = source["icon"];
	        this.description = source["description"];
	        this.system_prompt = source["system_prompt"];
	        this.sort_order = source["sort_order"];
	        this.builtin = source["builtin"];
	    }
	}
	export class Conversation {
	    id: string;
	    title: string;
	    provider: string;
	    model: string;
	    system_prompt: string;
	    agent_id: string;
	    mcp_server_ids: string;
	    starred: boolean;
	    mode: string;
	    knowledge_base_id: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Conversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.system_prompt = source["system_prompt"];
	        this.agent_id = source["agent_id"];
	        this.mcp_server_ids = source["mcp_server_ids"];
	        this.starred = source["starred"];
	        this.mode = source["mode"];
	        this.knowledge_base_id = source["knowledge_base_id"];
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
	export class KnowledgeBase {
	    id: string;
	    name: string;
	    description: string;
	    doc_count: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new KnowledgeBase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.doc_count = source["doc_count"];
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
	export class LLMModel {
	    id: string;
	    provider_id: string;
	    name: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider_id = source["provider_id"];
	        this.name = source["name"];
	        this.created_at = source["created_at"];
	    }
	}
	export class LLMProvider {
	    id: string;
	    name: string;
	    type: string;
	    api_key: string;
	    base_url: string;
	    enabled: boolean;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMProvider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.api_key = source["api_key"];
	        this.base_url = source["base_url"];
	        this.enabled = source["enabled"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class MCPServer {
	    id: string;
	    name: string;
	    type: string;
	    url: string;
	    command: string;
	    args: string;
	    env: string;
	    enabled: boolean;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPServer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.url = source["url"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.enabled = source["enabled"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class Message {
	    id: string;
	    conversation_id: string;
	    role: string;
	    content: string;
	    thinking?: string;
	    tool_calls?: string;
	    tool_result?: string;
	    attachments?: string;
	    agent_id: string;
	    mcp_server_ids: string;
	    mode: string;
	    knowledge_base_id: string;
	    generation_group_id: string;
	    gen_index: number;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversation_id = source["conversation_id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.thinking = source["thinking"];
	        this.tool_calls = source["tool_calls"];
	        this.tool_result = source["tool_result"];
	        this.attachments = source["attachments"];
	        this.agent_id = source["agent_id"];
	        this.mcp_server_ids = source["mcp_server_ids"];
	        this.mode = source["mode"];
	        this.knowledge_base_id = source["knowledge_base_id"];
	        this.generation_group_id = source["generation_group_id"];
	        this.gen_index = source["gen_index"];
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
	export class Setting {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new Setting(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class Skill {
	    id: string;
	    name: string;
	    description: string;
	    content: string;
	    enabled: boolean;
	    sort_order: number;
	
	    static createFrom(source: any = {}) {
	        return new Skill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.content = source["content"];
	        this.enabled = source["enabled"];
	        this.sort_order = source["sort_order"];
	    }
	}

}

