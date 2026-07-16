export function getThemeButtonActiveStyle(theme) {
  if (theme === 'light') return 'px-2.5 py-1 text-xs rounded transition-colors bg-white text-black font-bold shadow';
  if (theme === 'sepia') return 'px-2.5 py-1 text-xs rounded transition-colors bg-[#f4ecd8] text-[#5b4636] font-bold shadow';
  if (theme === 'warm') return 'px-2.5 py-1 text-xs rounded transition-colors bg-[#fbf0e3] text-[#5c4033] font-bold shadow';
  return 'px-2.5 py-1 text-xs rounded transition-colors bg-[#1a1a1a] text-white font-bold border border-black-300 shadow';
}

export function getThemeButtonInactiveStyle(theme) {
  return 'px-2.5 py-1 text-xs rounded transition-colors text-black-100 hover:text-white hover:bg-black-500';
}

export function getThemeRules(theme, font, lineHeight, margin) {
  let bg, fg;
  if (theme === 'light') {
    bg = '#ffffff';
    fg = '#000000';
  } else if (theme === 'sepia') {
    bg = '#f4ecd8';
    fg = '#5b4636';
  } else if (theme === 'warm') {
    bg = '#fbf0e3';
    fg = '#5c4033';
  } else {
    bg = '#1a1a1a';
    fg = '#e0e0e0';
  }
  return {
    body: {
      background: `${bg} !important`,
      color: `${fg} !important`,
      "font-family": `${font} !important`,
      "line-height": `${lineHeight} !important`,
      "padding": `0 ${margin} !important`
    },
    p: {
      color: `${fg} !important`
    }
  };
}

export function applyIframeStyles(rendition, theme, lineHeight) {
  if (!rendition) return;
  let bg, fg;
  if (theme === 'light') {
    bg = '#ffffff';
    fg = '#000000';
  } else if (theme === 'sepia') {
    bg = '#f4ecd8';
    fg = '#5b4636';
  } else if (theme === 'warm') {
    bg = '#fbf0e3';
    fg = '#5c4033';
  } else {
    bg = '#1a1a1a';
    fg = '#e0e0e0';
  }
  
  const lineSpacing = parseFloat(lineHeight);
  
  const rules = {
    '*': {
      'color': `${fg} !important`,
      'background-color': `${bg} !important`,
      'line-height': `${lineSpacing} !important`
    },
    'a': {
      'color': `${fg} !important`
    }
  };
  
  rendition.themes.default(rules);
}

export function applyTheme(theme, rendition, config, elements) {
  const { currentFont, currentLineHeight, currentMargin } = config;
  const { overlay, vContainer, cBody } = elements;
  
  if (theme === 'light') {
    if (vContainer) {
      vContainer.style.backgroundColor = '#ffffff';
      vContainer.style.color = '#000000';
    }
    if (cBody) cBody.style.backgroundColor = '#f5f5f5';
  } else if (theme === 'sepia') {
    if (vContainer) {
      vContainer.style.backgroundColor = '#f4ecd8';
      vContainer.style.color = '#5b4636';
    }
    if (cBody) cBody.style.backgroundColor = '#e9dfc4';
  } else if (theme === 'warm') {
    if (vContainer) {
      vContainer.style.backgroundColor = '#fbf0e3';
      vContainer.style.color = '#5c4033';
    }
    if (cBody) cBody.style.backgroundColor = '#eddccb';
  } else {
    if (vContainer) {
      vContainer.style.backgroundColor = '#1a1a1a';
      vContainer.style.color = '#e0e0e0';
    }
    if (cBody) cBody.style.backgroundColor = '#121212';
  }

  let bg, fg, border, headerBg, textMuted, inputBg;
  if (theme === 'light') {
    bg = '#ffffff';
    fg = '#111827';
    border = '#e5e7eb';
    headerBg = '#f3f4f6';
    textMuted = '#4b5563';
    inputBg = '#f9fafb';
  } else if (theme === 'sepia') {
    bg = '#f4ecd8';
    fg = '#5b4636';
    border = '#e6dcbd';
    headerBg = '#eae0c9';
    textMuted = '#705b4a';
    inputBg = '#fcf8ee';
  } else if (theme === 'warm') {
    bg = '#fbf0e3';
    fg = '#5c4033';
    border = '#eddccb';
    headerBg = '#f5e6d3';
    textMuted = '#785b4c';
    inputBg = '#fffaf4';
  } else {
    bg = '#1a1a1a';
    fg = '#e0e0e0';
    border = '#2d2d2d';
    headerBg = '#121212';
    textMuted = '#9ca3af';
    inputBg = '#262626';
  }

  if (overlay) {
    overlay.style.setProperty('--reader-bg', bg);
    overlay.style.setProperty('--reader-fg', fg);
    overlay.style.setProperty('--reader-border', border);
    overlay.style.setProperty('--reader-header-bg', headerBg);
    overlay.style.setProperty('--reader-text-muted', textMuted);
    overlay.style.setProperty('--reader-input-bg', inputBg);
  }
  
  if (rendition) {
    rendition.themes.register("light", getThemeRules("light", currentFont, currentLineHeight, currentMargin));
    rendition.themes.register("sepia", getThemeRules("sepia", currentFont, currentLineHeight, currentMargin));
    rendition.themes.register("warm", getThemeRules("warm", currentFont, currentLineHeight, currentMargin));
    rendition.themes.register("dark", getThemeRules("dark", currentFont, currentLineHeight, currentMargin));
    rendition.themes.select(theme);
    applyIframeStyles(rendition, theme, currentLineHeight);
  }
  
  document.querySelectorAll('[data-theme]').forEach(btn => {
    if (btn.getAttribute('data-theme') === theme) {
      btn.className = getThemeButtonActiveStyle(theme);
    } else {
      btn.className = getThemeButtonInactiveStyle(btn.getAttribute('data-theme'));
    }
  });
}
