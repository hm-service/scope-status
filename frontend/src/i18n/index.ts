// 自己写的极简 i18n：两张表 + 一个响应式 t()。
// 未命中的键原样返回键名（不做回退），漏配在界面上一眼可见。
import { ref } from 'vue'
import en from './en'
import zh, { type I18nKey } from './zh'

export type Lang = 'zh' | 'en'
export type { I18nKey }

const TABLES: Record<Lang, Record<I18nKey, string>> = { zh, en }
const LANG_STORAGE_KEY = 'lang'

export const LANGS: Lang[] = ['zh', 'en']

/** 默认按 navigator.language：zh* → 简体中文，其余 → English。 */
function detectLang(): Lang {
  const preferred = typeof navigator === 'undefined' ? '' : navigator.language || ''
  return preferred.toLowerCase().startsWith('zh') ? 'zh' : 'en'
}

function storedLang(): Lang | null {
  try {
    const saved = localStorage.getItem(LANG_STORAGE_KEY)
    return saved === 'zh' || saved === 'en' ? saved : null
  } catch {
    return null
  }
}

export const lang = ref<Lang>(storedLang() || detectLang())

/** 把语言同步到 <html lang>；persist 只在用户手动切换时为真。 */
export function setLang(next: Lang, persist = false): void {
  lang.value = next
  document.documentElement.lang = next === 'zh' ? 'zh-CN' : 'en'
  if (persist) {
    try {
      localStorage.setItem(LANG_STORAGE_KEY, next)
    } catch {
      /* 存不下就算了，本次会话里照常生效 */
    }
  }
}

export type TParams = Record<string, string | number>

/** 取一条文案，替换 {name} 占位符；没有这个键就返回键名本身。 */
export function t(key: I18nKey, params?: TParams): string {
  const template = TABLES[lang.value][key]
  if (template === undefined) return key
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (match, name: string) =>
    Object.prototype.hasOwnProperty.call(params, name) ? String(params[name]) : match,
  )
}
