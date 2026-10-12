// 数字 / 体积的小格式化，单位不翻译。

export function latencyText(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return ''
  if (ms < 1) return '<1 ms'
  return `${Math.round(ms)} ms`
}

/** 本轮探测耗时：毫秒或秒（保留两位）。 */
export function formatDuration(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return '–'
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)} s` : `${Math.round(ms)} ms`
}

export function formatSize(bytes: number | null | undefined): string {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value >= 10 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`
}
