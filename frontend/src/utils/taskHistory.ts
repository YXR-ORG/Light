export function shouldShowTaskHistory(historyCount: number, _completedRoundCount = 0): boolean {
  return historyCount > 0
}

export interface TaskHistoryMsgLike {
  role: string
  content: string
}

export interface TaskRoundLike {
  userContent: string
  assistantContent: string
}

function normalizeText(s: string | undefined): string {
  return (s || '').trim()
}

/**
 * 去掉 DB 历史中已被内存 completedRounds / 当前流式用户消息覆盖的尾部轮次。
 * 切换会话回来时两者会同时存在，直接并排渲染会看到同一轮问答出现两次。
 * 优先保留 completedRounds（含推理链），DB 只补更早且内存没有的部分。
 */
export function filterOverlappingTaskHistory<T extends TaskHistoryMsgLike>(
  history: T[],
  rounds: TaskRoundLike[],
  streamingUserContent = '',
): T[] {
  if (!history.length) return history

  let end = history.length

  for (let i = rounds.length - 1; i >= 0; i--) {
    if (end < 2) break
    const round = rounds[i]
    const assistant = history[end - 1]
    const user = history[end - 2]
    if (
      user.role === 'user' &&
      assistant.role === 'assistant' &&
      normalizeText(user.content) === normalizeText(round.userContent) &&
      normalizeText(assistant.content) === normalizeText(round.assistantContent)
    ) {
      end -= 2
      continue
    }
    // completedRounds 应对应历史尾部；一旦对不上就停止，避免误删更早轮次
    break
  }

  const streamUser = normalizeText(streamingUserContent)
  if (streamUser && end >= 1) {
    const last = history[end - 1]
    if (last.role === 'user' && normalizeText(last.content) === streamUser) {
      end -= 1
    }
  }

  return end === history.length ? history : history.slice(0, end)
}
