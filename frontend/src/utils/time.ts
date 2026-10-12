// 时间显示：相对时间 + 绝对时间（精确到秒）。格式随语言走，同一页面内保持一致。
import { ref } from 'vue'
import { lang, t } from '../i18n'

/** 相对时间的刷新节拍：每 30 秒走一格，界面上的「刚刚 / N 分钟前」自己会变老。 */
export const now = ref(Date.now())

export function tickNow(): void {
  now.value = Date.now()
}

const pad2 = (value: number): string => String(value).padStart(2, '0')

function locale(): string {
  return lang.value === 'zh' ? 'zh-CN' : 'en-US'
}

/** 精确到秒的时钟：22:45:11 */
export function clockText(date: Date): string {
  return new Intl.DateTimeFormat(locale(), {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}

/** 顶栏「最近推送」用的时刻。 */
export function formatTime(date: Date = new Date()): string {
  return clockText(date)
}

/** 精确到秒的本地时间：中文 2026-10-12 23:24:10，英文 10/12/2026 23:24:10 */
export function fullStamp(ms: number): string {
  const date = new Date(ms)
  const clock = clockText(date)
  if (locale() === 'zh-CN') {
    return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ${clock}`
  }
  const day = new Intl.DateTimeFormat('en-US', { year: 'numeric', month: '2-digit', day: '2-digit' }).format(date)
  return `${day} ${clock}`
}

/** 概况条里的紧凑写法：今天只给时分秒，同年省掉年份 */
export function shortStamp(ms: number): string {
  const date = new Date(ms)
  const today = new Date()
  const clock = clockText(date)
  if (date.toDateString() === today.toDateString()) return clock
  if (date.getFullYear() === today.getFullYear()) {
    if (locale() === 'zh-CN') return `${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ${clock}`
    const day = new Intl.DateTimeFormat('en-US', { month: 'short', day: '2-digit' }).format(date)
    return `${day} ${clock}`
  }
  return fullStamp(ms)
}

/** 相对时间：给「新不新」，不关心具体时刻。 */
export function relativeTime(ms: number): string {
  const diff = now.value - ms
  if (diff < 60_000) return t('time.justNow')
  if (diff < 3_600_000) return t('time.minutesAgo', { count: Math.floor(diff / 60_000) })
  if (diff < 86_400_000) return t('time.hoursAgo', { count: Math.floor(diff / 3_600_000) })
  if (diff < 30 * 86_400_000) return t('time.daysAgo', { count: Math.floor(diff / 86_400_000) })
  if (diff < 365 * 86_400_000) return t('time.monthsAgo', { count: Math.floor(diff / (30 * 86_400_000)) })
  return t('time.yearsAgo', { count: Math.floor(diff / (365 * 86_400_000)) })
}
