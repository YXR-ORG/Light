export function shouldShowTaskHistory(historyCount: number, _completedRoundCount = 0): boolean {
  return historyCount > 0
}
