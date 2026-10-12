// 图标一律内联 SVG：不依赖系统 emoji 字体。字符串来自旧实现，原样搬过来。
export const ICONS = {
  chevron:
    '<svg viewBox="0 0 16 16" width="13" height="13" aria-hidden="true"><path d="M6 3.5 10.5 8 6 12.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>',
  globe:
    '<svg viewBox="0 0 20 20" width="15" height="15" aria-hidden="true"><circle cx="10" cy="10" r="7.2" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M2.8 10h14.4M10 2.8c1.9 2 3 4.5 3 7.2s-1.1 5.2-3 7.2c-1.9-2-3-4.5-3-7.2s1.1-5.2 3-7.2Z" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>',
  lock:
    '<svg viewBox="0 0 20 20" width="15" height="15" aria-hidden="true"><rect x="4.2" y="8.6" width="11.6" height="8" rx="2" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M7 8.6V6.8a3 3 0 0 1 6 0v1.8" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>',
  plus:
    '<svg viewBox="0 0 20 20" width="15" height="15" aria-hidden="true"><path d="M10 4.5v11M4.5 10h11" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>',
  paperclip:
    '<svg viewBox="0 0 20 20" width="14" height="14" aria-hidden="true"><path d="M13.6 6.4 7.9 12.1a1.9 1.9 0 0 0 2.7 2.7l5.1-5.1a3.6 3.6 0 0 0-5.1-5.1L5.2 9.9a5.2 5.2 0 0 0 7.4 7.4" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
  download:
    '<svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true"><path d="M10 3.6v8.2m0 0 3-3m-3 3-3-3M4.4 15.4h11.2" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>',
  file:
    '<svg viewBox="0 0 20 20" width="17" height="17" aria-hidden="true"><path d="M5.5 2.8h5.2l4 4v10.4H5.5z" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/><path d="M10.6 2.9v4h4" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/></svg>',
  pencil:
    '<svg viewBox="0 0 20 20" width="14" height="14" aria-hidden="true"><path d="m13.4 3.6 3 3L7.6 15.4l-3.9.9.9-3.9z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/></svg>',
  trash:
    '<svg viewBox="0 0 20 20" width="14" height="14" aria-hidden="true"><path d="M4.6 6.2h10.8M8.2 6.2V4.4h3.6v1.8M6.2 6.2l.7 9.4h6.2l.7-9.4" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
  close:
    '<svg viewBox="0 0 20 20" width="14" height="14" aria-hidden="true"><path d="m6 6 8 8M14 6l-8 8" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"/></svg>',
  eye:
    '<svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true"><path d="M2.4 10S5.6 4.8 10 4.8 17.6 10 17.6 10 14.4 15.2 10 15.2 2.4 10 2.4 10Z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><circle cx="10" cy="10" r="2.3" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>',
  eyeOff:
    '<svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true"><path d="M3.4 3.4 16.6 16.6" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/><path d="M7.7 5.4A8.9 8.9 0 0 1 10 5c4.4 0 7.6 5 7.6 5a17 17 0 0 1-2.9 3.3M5.2 6.8A16.6 16.6 0 0 0 2.4 10s3.2 5 7.6 5c.9 0 1.7-.2 2.4-.5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
} as const

export const THEME_ICONS = {
  dark: '<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path d="M20.5 14.6A8.5 8.5 0 0 1 9.4 3.5a8.5 8.5 0 1 0 11.1 11.1Z" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/></svg>',
  light:
    '<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><circle cx="12" cy="12" r="4.2" fill="none" stroke="currentColor" stroke-width="1.8"/><path d="M12 2.8v2.4M12 18.8v2.4M2.8 12h2.4M18.8 12h2.4M5.5 5.5l1.7 1.7M16.8 16.8l1.7 1.7M18.5 5.5l-1.7 1.7M7.2 16.8l-1.7 1.7" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>',
} as const
