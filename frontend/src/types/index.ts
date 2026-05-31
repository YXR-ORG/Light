export interface Conversation {
  id: string
  title: string
  provider: string
  model: string
  created_at: string
  updated_at: string
}

export interface Message {
  id: string
  conversation_id: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  content: string
  tool_calls?: string
  tool_result?: string
  created_at: string
}

export interface Setting {
  key: string
  value: string
}

export interface StreamChunk {
  content: string
  thinking?: string
  done: boolean
  error?: string
}
