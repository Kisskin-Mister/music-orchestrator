const ACCENT_STORAGE_KEY = 'mo_accent';

// Preset hues; actual lightness/chroma is clamped in CSS (--accent, styles.css)
// so any pick — preset or custom — stays readable against the fixed --accent-ink text color.
export const ACCENT_PRESETS = [
  { hex: '#b8f545', name: 'Лайм' },
  { hex: '#45d8f5', name: 'Циан' },
  { hex: '#f545c8', name: 'Маджента' },
  { hex: '#f5a545', name: 'Янтарь' },
  { hex: '#5a8bff', name: 'Синий' },
  { hex: '#ff6b5f', name: 'Коралл' },
];
export const DEFAULT_ACCENT = ACCENT_PRESETS[0].hex;

export function applyAccent(hex: string) {
  document.documentElement.style.setProperty('--accent-raw', hex);
  window.localStorage.setItem(ACCENT_STORAGE_KEY, hex);
}

export function initAccent() {
  const saved = window.localStorage.getItem(ACCENT_STORAGE_KEY);
  if (saved) document.documentElement.style.setProperty('--accent-raw', saved);
}

export function getStoredAccent() {
  return window.localStorage.getItem(ACCENT_STORAGE_KEY) || DEFAULT_ACCENT;
}
