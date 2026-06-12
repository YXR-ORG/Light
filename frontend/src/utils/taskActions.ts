export interface TaskStepLike { type: string; content?: string }

export function shouldShowTaskActions(role: 'user' | 'assistant', streaming: boolean | undefined, hasContent: boolean): boolean {
  return role === 'assistant' && !streaming && hasContent
}

export function taskCopyText(finalContent: string, steps: TaskStepLike[]): string {
  if (finalContent) return finalContent
  return steps.filter(step => step.type === 'content').map(step => step.content || '').join('')
}
