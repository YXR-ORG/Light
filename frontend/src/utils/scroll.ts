export function isNearBottom(scrollHeight: number, scrollTop: number, clientHeight: number, threshold = 60): boolean {
  return scrollHeight - scrollTop - clientHeight < threshold
}

export function shouldAutoScroll(force: boolean, userScrolled: boolean): boolean {
  return force || !userScrolled
}
