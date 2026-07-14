import { request, resolvePath } from './api.js';

const loadedLibraries = {};

function loadScript(src) {
  if (loadedLibraries[src]) return loadedLibraries[src];
  loadedLibraries[src] = new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = src;
    script.onload = () => resolve();
    script.onerror = (err) => {
      delete loadedLibraries[src];
      reject(err);
    };
    document.head.appendChild(script);
  });
  return loadedLibraries[src];
}

function getThemeButtonActiveStyle(theme) {
  if (theme === 'light') return 'px-2.5 py-1 text-xs rounded transition-colors bg-white text-black font-bold shadow';
  if (theme === 'sepia') return 'px-2.5 py-1 text-xs rounded transition-colors bg-[#f4ecd8] text-[#5b4636] font-bold shadow';
  if (theme === 'warm') return 'px-2.5 py-1 text-xs rounded transition-colors bg-[#fbf0e3] text-[#5c4033] font-bold shadow';
  return 'px-2.5 py-1 text-xs rounded transition-colors bg-[#1a1a1a] text-white font-bold border border-black-300 shadow';
}

function getThemeButtonInactiveStyle(theme) {
  return 'px-2.5 py-1 text-xs rounded transition-colors text-black-100 hover:text-white hover:bg-black-500';
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;");
}

/**
 * Opens the built-in reader for an e-book.
 * @param {object} item - The library item object.
 * @param {string} token - The active auth token.
 */
export async function openEbookReader(item, token) {
  const itemId = item.id;
  const title = item.media?.metadata?.title || item.title || "Untitled";
  const author = item.media?.metadata?.authorName || "Unknown Author";
  
  // Resolve format
  let format = item.media?.ebookFormat || "";
  if (!format && item.media?.ebookFile) {
    format = item.media.ebookFile.ebookFormat || "";
    if (!format && item.media.ebookFile.metadata?.ext) {
      format = item.media.ebookFile.metadata.ext.toLowerCase().replace('.', '');
    }
  }
  if (!format && item.libraryFiles) {
    const ebookFile = item.libraryFiles.find(f => f.fileType === 'ebook');
    if (ebookFile) {
      format = ebookFile.ebookFormat || (ebookFile.ext ? ebookFile.ext.toLowerCase().replace('.', '') : "");
    }
  }
  format = format.toLowerCase().trim();

  // Create reader overlay
  const overlay = document.createElement('div');
  overlay.id = 'built-in-reader-overlay';
  overlay.className = 'fixed inset-0 bg-bg z-60 flex flex-col font-sans select-none';
  overlay.style.zIndex = '1000';
  overlay.innerHTML = `
    <!-- Header Bar -->
    <div class="h-16 bg-primary border-b border-black-600/50 flex items-center justify-between px-6 z-50 flex-shrink-0">
      <div class="flex items-center space-x-3 truncate mr-4">
        <button id="reader-close-btn" class="flex items-center space-x-1.5 text-sm text-black-50 hover:text-white transition-colors focus:outline-none">
          <span class="material-symbols text-xl">arrow_back</span>
          <span class="hidden sm:inline">Back</span>
        </button>
        <div class="h-5 w-px bg-black-600"></div>
        <div class="truncate">
          <h3 class="text-sm font-bold text-white truncate" id="reader-book-title"></h3>
          <p class="text-xs text-black-50 truncate" id="reader-book-author"></p>
        </div>
      </div>
      
      <!-- Controls (Theme, Font Size, TOC) -->
      <div class="flex items-center space-x-4 flex-shrink-0">
        <!-- EPUB-only controls -->
        <div id="epub-controls" class="hidden flex items-center space-x-3">
          <!-- Table of Contents Trigger -->
          <button id="reader-toc-btn" class="p-2 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors focus:outline-none" title="Table of Contents">
            <span class="material-symbols text-xl">menu</span>
          </button>
          
          <!-- Text-To-Speech Trigger -->
          <button id="reader-tts-btn" class="p-2 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors focus:outline-none" title="Text-to-Speech (Read Aloud)">
            <span class="material-symbols text-xl">volume_up</span>
          </button>

          <!-- Font Size Decrease -->
          <button id="reader-font-dec-btn" class="p-2 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors focus:outline-none" title="Decrease Font Size">
            <span class="text-xs font-bold font-mono">A-</span>
          </button>
          <!-- Font Size Increase -->
          <button id="reader-font-inc-btn" class="p-2 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors focus:outline-none" title="Increase Font Size">
            <span class="text-sm font-bold font-mono">A+</span>
          </button>
          
          <!-- Theme Selector -->
          <div class="flex items-center bg-black-600 border border-black-400 rounded p-0.5 space-x-0.5">
            <button id="theme-light-btn" class="px-2.5 py-1 text-xs rounded transition-colors text-black-100 hover:text-white" data-theme="light">Light</button>
            <button id="theme-sepia-btn" class="px-2.5 py-1 text-xs rounded transition-colors text-black-100 hover:text-white" data-theme="sepia">Sepia</button>
            <button id="theme-warm-btn" class="px-2.5 py-1 text-xs rounded transition-colors text-black-100 hover:text-white" data-theme="warm">Warm</button>
            <button id="theme-dark-btn" class="px-2.5 py-1 text-xs rounded transition-colors text-black-100 hover:text-white" data-theme="dark">Dark</button>
          </div>

          <!-- Typography / Reader Settings Trigger -->
          <button id="reader-settings-btn" class="p-2 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors focus:outline-none" title="Typography & Page Layout Settings">
            <span class="material-symbols text-xl">settings</span>
          </button>
        </div>

        <div class="h-5 w-px bg-black-600"></div>
        <span class="bg-accent/10 border border-accent/20 text-accent px-2 py-0.5 rounded text-xs font-semibold uppercase tracking-wider" id="reader-format-badge"></span>
      </div>
    </div>

    <!-- Reader Settings Popover -->
    <div id="reader-settings-popover" class="hidden absolute right-6 top-16 bg-[#1a1a1a]/95 border border-black-400 rounded-lg shadow-2xl p-4 z-50 w-72 backdrop-blur-md space-y-4 text-white font-sans">
      <div class="border-b border-black-600/50 pb-2 mb-2">
        <h4 class="text-xs font-bold text-white uppercase tracking-wider">Reader Display Settings</h4>
      </div>
      
      <!-- Font Family Choice -->
      <div class="space-y-1.5">
        <label class="text-xs text-black-100 font-semibold">Font Style</label>
        <select id="reader-font-family-select" class="w-full bg-black-600 text-white text-xs px-2 py-1.5 rounded border border-black-400 focus:outline-none focus:border-accent font-sans">
          <option value="Georgia, serif">Georgia (Default)</option>
          <option value="'Merriweather', serif">Merriweather (Serif)</option>
          <option value="'Inter', sans-serif">Inter (Sans-Serif)</option>
          <option value="'Arial', sans-serif">Arial / Sans-Serif</option>
          <option value="'OpenDyslexic', sans-serif">OpenDyslexic</option>
        </select>
      </div>

      <!-- Line Spacing Choice -->
      <div class="space-y-1.5">
        <label class="text-xs text-black-100 font-semibold">Line Spacing</label>
        <div class="grid grid-cols-4 gap-1 bg-black-600 p-0.5 rounded border border-black-400">
          <button class="reader-line-spacing-btn text-xs py-1 text-center rounded transition-colors" data-val="1.25">1.25</button>
          <button class="reader-line-spacing-btn text-xs py-1 text-center rounded transition-colors" data-val="1.5">1.5</button>
          <button class="reader-line-spacing-btn text-xs py-1 text-center rounded transition-colors" data-val="1.75">1.75</button>
          <button class="reader-line-spacing-btn text-xs py-1 text-center rounded transition-colors" data-val="2.0">2.0</button>
        </div>
      </div>

      <!-- Page Margin Choice -->
      <div class="space-y-1.5">
        <label class="text-xs text-black-100 font-semibold">Side Margins</label>
        <div class="grid grid-cols-3 gap-1 bg-black-600 p-0.5 rounded border border-black-400">
          <button class="reader-margin-btn text-xs py-1 text-center rounded transition-colors" data-val="5%">Narrow</button>
          <button class="reader-margin-btn text-xs py-1 text-center rounded transition-colors" data-val="15%">Medium</button>
          <button class="reader-margin-btn text-xs py-1 text-center rounded transition-colors" data-val="25%">Wide</button>
        </div>
      </div>

      <!-- Flow Layout choice -->
      <div class="space-y-1.5">
        <label class="text-xs text-black-100 font-semibold">Layout Flow</label>
        <div class="grid grid-cols-2 gap-1 bg-black-600 p-0.5 rounded border border-black-400">
          <button id="reader-flow-paginated-btn" class="text-xs py-1 text-center rounded transition-colors" data-val="paginated">Paginated</button>
          <button id="reader-flow-scrolled-btn" class="text-xs py-1 text-center rounded transition-colors" data-val="scrolled-doc">Continuous</button>
        </div>
      </div>

      <!-- Reader Layout choice -->
      <div class="space-y-1.5" id="reader-page-layout-section">
        <label class="text-xs text-black-100 font-semibold">Page Layout</label>
        <div class="grid grid-cols-2 gap-1 bg-black-600 p-0.5 rounded border border-black-400">
          <button id="reader-layout-single-btn" class="text-xs py-1 text-center rounded transition-colors" data-val="none">Single Page</button>
          <button id="reader-layout-spread-btn" class="text-xs py-1 text-center rounded transition-colors" data-val="auto">Two Pages</button>
        </div>
      </div>
    </div>

    <!-- TTS Controller Popover -->
    <div id="reader-tts-popover" class="hidden absolute right-16 top-16 bg-[#1a1a1a]/95 border border-black-400 rounded-lg shadow-2xl p-4 z-50 w-72 backdrop-blur-md space-y-4 text-white font-sans">
      <div class="border-b border-black-600/50 pb-2 mb-2 flex justify-between items-center">
        <h4 class="text-xs font-bold text-white uppercase tracking-wider flex items-center space-x-1">
          <span class="material-symbols text-sm">volume_up</span>
          <span>Text-To-Speech</span>
        </h4>
        <button id="close-tts-btn" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-sm">close</span>
        </button>
      </div>

      <!-- Play / Pause / Stop controls -->
      <div class="flex items-center justify-center space-x-3">
        <button id="tts-prev-sentence-btn" class="p-1.5 bg-black-600 border border-black-400 hover:bg-black-500 rounded transition-colors text-white" title="Previous Sentence">
          <span class="material-symbols text-lg">skip_previous</span>
        </button>
        <button id="tts-play-btn" class="p-2.5 bg-accent text-primary rounded-full hover:opacity-90 transition-opacity font-bold shadow" title="Play / Pause">
          <span class="material-symbols text-xl" id="tts-play-icon">play_arrow</span>
        </button>
        <button id="tts-stop-btn" class="p-1.5 bg-black-600 border border-black-400 hover:bg-black-500 rounded transition-colors text-white" title="Stop">
          <span class="material-symbols text-lg">stop</span>
        </button>
        <button id="tts-next-sentence-btn" class="p-1.5 bg-black-600 border border-black-400 hover:bg-black-500 rounded transition-colors text-white" title="Next Sentence">
          <span class="material-symbols text-lg">skip_next</span>
        </button>
      </div>

      <!-- Speech Speed -->
      <div class="space-y-1">
        <div class="flex justify-between text-xs text-black-100 font-semibold">
          <span>Speed</span>
          <span id="tts-speed-val">1.0x</span>
        </div>
        <input type="range" id="tts-speed-slider" min="0.5" max="2.0" step="0.1" value="1.0" class="w-full accent-accent bg-black-600 h-1 rounded" />
      </div>

      <!-- Voice Selector -->
      <div class="space-y-1.5">
        <label class="text-xs text-black-100 font-semibold">Voice</label>
        <select id="tts-voice-select" class="w-full bg-black-600 text-white text-xs px-2 py-1.5 rounded border border-black-400 focus:outline-none focus:border-accent font-sans">
          <!-- Voice options populated dynamically -->
        </select>
      </div>
    </div>

    <!-- Highlight Modal -->
    <div id="reader-highlight-modal" class="hidden fixed inset-0 bg-black/60 flex items-center justify-center z-70">
      <div class="bg-primary border border-black-400 rounded-lg shadow-2xl p-4 w-96 max-w-[90vw] text-white space-y-4">
        <h4 class="text-sm font-bold border-b border-black-600/50 pb-2">Add Highlight / Bookmark</h4>
        
        <div class="space-y-1">
          <label class="text-xs text-black-100 font-semibold">Selected Text</label>
          <div id="highlight-selected-text" class="text-xs bg-black-600 p-2 rounded max-h-24 overflow-y-auto italic text-black-50 border border-black-400"></div>
        </div>

        <div class="space-y-1">
          <label class="text-xs text-black-100 font-semibold">Note / Thoughts</label>
          <textarea id="highlight-note-input" class="w-full bg-black-600 text-white text-xs px-2 py-1.5 rounded border border-black-400 focus:outline-none focus:border-accent h-16 resize-none" placeholder="Add custom note..."></textarea>
        </div>

        <div class="space-y-1">
          <label class="text-xs text-black-100 font-semibold">Highlight Color</label>
          <div class="flex items-center space-x-2">
            <button class="hl-color-btn w-6 h-6 rounded-full border border-black-300 transition-transform hover:scale-110" data-color="#ffeb3b" style="background-color: #ffeb3b;" title="Yellow"></button>
            <button class="hl-color-btn w-6 h-6 rounded-full border border-black-300 transition-transform hover:scale-110" data-color="#8bc34a" style="background-color: #8bc34a;" title="Green"></button>
            <button class="hl-color-btn w-6 h-6 rounded-full border border-black-300 transition-transform hover:scale-110" data-color="#f48fb1" style="background-color: #f48fb1;" title="Pink"></button>
            <button class="hl-color-btn w-6 h-6 rounded-full border border-black-300 transition-transform hover:scale-110" data-color="#29b6f6" style="background-color: #29b6f6;" title="Blue"></button>
          </div>
        </div>

        <div class="flex justify-end space-x-2 pt-2">
          <button id="highlight-cancel-btn" class="bg-black-400 hover:bg-black-300 text-white font-semibold px-4 py-2 rounded text-xs transition-colors">Cancel</button>
          <button id="highlight-save-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity shadow">Save</button>
        </div>
      </div>
    </div>

    <!-- Center Content Area -->
    <div class="flex-grow flex relative min-h-0 bg-[#121212]" id="reader-main-viewport">
      <!-- Sidebar / Table of Contents Drawer -->
      <div id="reader-toc-drawer" class="absolute left-0 top-0 bottom-0 w-80 bg-primary border-r border-black-600/50 shadow-2xl z-40 flex flex-col" style="transform: translateX(-100%); transition: transform 0.3s ease; max-width: 85vw;">
        <div class="p-2 border-b border-black-600/50 flex flex-col flex-shrink-0">
          <div class="flex justify-between items-center px-2 py-1">
            <h4 class="font-bold text-xs text-white uppercase tracking-wider">Reader Navigation</h4>
            <button id="close-toc-btn" class="text-black-100 hover:text-white transition-colors focus:outline-none">
              <span class="material-symbols text-lg">close</span>
            </button>
          </div>
          <div class="flex mt-1 bg-black-600 p-0.5 rounded border border-black-400">
            <button id="drawer-tab-chapters" class="flex-1 text-center py-1 text-xs rounded transition-colors bg-accent text-primary font-bold shadow">Chapters</button>
            <button id="drawer-tab-bookmarks" class="flex-1 text-center py-1 text-xs rounded transition-colors text-black-100 hover:text-white">Bookmarks</button>
          </div>
        </div>
        <!-- Chapters list -->
        <div id="reader-toc-list" class="flex-grow overflow-y-auto p-2 space-y-1 no-scroll">
          <!-- Table of Contents items go here -->
        </div>
        <!-- Bookmarks list -->
        <div id="reader-bookmarks-list" class="hidden flex-grow overflow-y-auto p-2 space-y-2 no-scroll">
          <!-- Bookmarks and highlights go here -->
        </div>
      </div>

      <!-- EPUB Left Arrow (overlay hover) -->
      <button id="epub-prev-page-btn" class="hidden absolute left-0 top-0 bottom-0 w-16 bg-gradient-to-r from-black/20 to-transparent flex items-center justify-center opacity-0 hover:opacity-100 transition-opacity z-30 cursor-pointer text-white border-none focus:outline-none">
        <span class="material-symbols text-4xl">chevron_left</span>
      </button>

      <!-- Reader Rendering Viewport -->
      <div class="flex-grow h-full w-full flex items-center justify-center p-4 min-w-0" id="reader-content-body">
        <!-- Loading indicator -->
        <div class="flex flex-col items-center space-y-3" id="reader-loading-spinner">
          <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent"></div>
          <p class="text-xs text-black-100 font-medium">Loading built-in reader...</p>
        </div>
      </div>

      <!-- EPUB Right Arrow (overlay hover) -->
      <button id="epub-next-page-btn" class="hidden absolute right-0 top-0 bottom-0 w-16 bg-gradient-to-l from-black/20 to-transparent flex items-center justify-center opacity-0 hover:opacity-100 transition-opacity z-30 cursor-pointer text-white border-none focus:outline-none">
        <span class="material-symbols text-4xl">chevron_right</span>
      </button>
    </div>

    <!-- Footer / Progress Bar -->
    <div class="h-12 bg-primary border-t border-black-600/50 flex items-center justify-between px-6 z-50 flex-shrink-0" id="reader-footer">
      <div class="flex items-center space-x-2 text-xs text-black-50">
        <span id="reader-progress-percent">Progress: 0%</span>
      </div>
      
      <!-- Timeline/Seeker -->
      <div class="flex-grow max-w-lg mx-8 hidden sm:block">
        <div class="w-full bg-black-600 h-1.5 rounded-full overflow-hidden border border-black-300">
          <div id="reader-progress-bar-fill" class="bg-accent h-full w-0 transition-all duration-300"></div>
        </div>
      </div>

      <div class="text-xs text-black-100" id="epub-page-info">
        <!-- Page location info -->
      </div>
    </div>
  `;

  document.body.appendChild(overlay);

  // Set Title / Author / Format
  document.getElementById('reader-book-title').textContent = title;
  document.getElementById('reader-book-author').textContent = author;
  document.getElementById('reader-format-badge').textContent = format || 'E-BOOK';

  // Reader variables
  let book = null;
  let rendition = null;
  let currentTheme = 'dark';
  let currentFontSize = 100;
  let currentFont = 'Georgia, serif';
  let currentLineHeight = '1.5';
  let currentMargin = '15%';
  let currentLayout = 'auto'; // 'none' for single, 'auto' for two-page spread
  let currentFlow = 'paginated'; // 'paginated' or 'scrolled-doc'
  let progressSaveTimeout = null;
  let currentProgress = 0;
  let tocDrawerOpen = false;

  // Selected Text & Highlight Variables
  let selectedCfiRange = null;
  let selectedTextStr = "";
  let activeColor = "#ffeb3b";

  // TTS Variables
  let ttsUtterance = null;
  let ttsSentences = [];
  let currentSentenceIdx = 0;
  let isTtsPlaying = false;
  let ttsRate = 1.0;
  let selectedVoiceName = "";

  const activeBtnClass = "bg-accent text-primary font-bold shadow";
  const inactiveBtnClass = "text-black-100 hover:text-white hover:bg-black-500 bg-black-600";

  // Save Settings helper
  const saveSettings = () => {
    localStorage.setItem('ereaderSettings', JSON.stringify({
      theme: currentTheme,
      fontScale: currentFontSize,
      fontFamily: currentFont,
      lineHeight: currentLineHeight,
      margin: currentMargin,
      layout: currentLayout,
      flow: currentFlow
    }));
  };

  // Close Reader logic
  const closeReader = () => {
    // Clean up timeouts
    if (progressSaveTimeout) {
      clearTimeout(progressSaveTimeout);
    }
    
    // Save final progress immediately if EPUB
    if (rendition && rendition.location && format === 'epub') {
      const cfi = rendition.location.start.cfi;
      let pct = currentProgress;
      if (book && book.locations && book.locations.total > 0) {
        pct = book.locations.percentageFromCfi(cfi) || currentProgress;
      }
      request('PATCH', `/api/me/progress/${itemId}`, {
        ebookLocation: cfi,
        ebookProgress: pct
      }).catch(err => console.error("Error saving final progress:", err));
    }

    // Stop TTS speaking
    if ('speechSynthesis' in window) {
      window.speechSynthesis.cancel();
    }

    // Unbind event listeners
    document.removeEventListener('keyup', keyListener);
    window.removeEventListener('resize', handleResize);
    document.removeEventListener('click', clickOutsideTOC);
    document.removeEventListener('click', clickOutsideSettings);
    document.removeEventListener('click', clickOutsideTts);

    // Remove overlay
    overlay.remove();
  };

  // Bind close buttons
  document.getElementById('reader-close-btn').onclick = closeReader;

  // Keyboard navigation
  const keyListener = (e) => {
    if (!rendition) return;
    if (e.key === "ArrowLeft") {
      rendition.prev();
    } else if (e.key === "ArrowRight") {
      rendition.next();
    }
  };

  // Resize handler
  const handleResize = () => {
    if (rendition) {
      rendition.resize();
    }
    // Auto-close TOC drawer if window is resized smaller (e.g. less than 768px wide)
    if (window.innerWidth < 768 && tocDrawerOpen) {
      closeTOCDrawer();
    }
  };

  // TOC drawer handlers
  const drawer = document.getElementById('reader-toc-drawer');
  const openTOCDrawer = () => {
    drawer.style.transform = 'translateX(0)';
    tocDrawerOpen = true;
  };
  const closeTOCDrawer = () => {
    drawer.style.transform = 'translateX(-100%)';
    tocDrawerOpen = false;
  };
  
  document.getElementById('reader-toc-btn').onclick = (e) => {
    e.stopPropagation();
    if (tocDrawerOpen) {
      closeTOCDrawer();
    } else {
      openTOCDrawer();
    }
  };
  document.getElementById('close-toc-btn').onclick = closeTOCDrawer;

  const clickOutsideTOC = (e) => {
    if (!drawer.contains(e.target) && e.target.id !== 'reader-toc-btn' && !e.target.closest('#reader-toc-btn')) {
      closeTOCDrawer();
    }
  };

  // Load saved settings
  try {
    const saved = localStorage.getItem('ereaderSettings');
    if (saved) {
      const parsed = JSON.parse(saved);
      if (parsed.theme) currentTheme = parsed.theme;
      if (parsed.fontScale) currentFontSize = parsed.fontScale;
      if (parsed.fontFamily) currentFont = parsed.fontFamily;
      if (parsed.lineHeight) currentLineHeight = parsed.lineHeight;
      if (parsed.margin) currentMargin = parsed.margin;
      if (parsed.layout) currentLayout = parsed.layout;
      if (parsed.flow) currentFlow = parsed.flow;
    }
  } catch (err) {
    console.error("Failed to load ereader settings:", err);
  }

  // Load ebook content based on format
  const contentBody = document.getElementById('reader-content-body');
  const spinner = document.getElementById('reader-loading-spinner');

  if (format === 'pdf') {
    // Hide EPUB-specific controls and elements
    document.getElementById('epub-controls').classList.add('hidden');
    document.getElementById('reader-footer').classList.add('hidden');
    
    contentBody.innerHTML = `
      <div class="flex flex-col items-center justify-center h-full w-full space-y-3" id="pdf-loading-indicator">
        <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent"></div>
        <p class="text-xs text-black-100 font-medium">Initializing custom PDF viewer...</p>
      </div>
    `;

    try {
      // Dynamic load PDF.js from cdnjs
      await loadScript('https://cdnjs.cloudflare.com/ajax/libs/pdf.js/3.4.120/pdf.min.js');
      pdfjsLib.GlobalWorkerOptions.workerSrc = 'https://cdnjs.cloudflare.com/ajax/libs/pdf.js/3.4.120/pdf.worker.min.js';

      const pdfUrl = resolvePath(`/api/items/${itemId}/ebook?token=${token}`);
      const pdfDoc = await pdfjsLib.getDocument(pdfUrl).promise;

      // Render the viewer interface inside contentBody
      contentBody.innerHTML = `
        <div class="flex h-full w-full relative min-w-0" id="pdf-viewer-container">
          <!-- Left Thumbnails Sidebar / Rail -->
          <div id="pdf-thumbnails-sidebar" class="w-44 bg-primary border-r border-black-600/50 flex flex-col flex-shrink-0 z-30 select-none">
            <div class="p-3 border-b border-black-600/50 flex items-center justify-between">
              <span class="text-[10px] font-bold text-white uppercase tracking-wider">Thumbnails</span>
              <span id="pdf-total-pages-badge" class="text-[9px] bg-black-600 px-1.5 py-0.5 rounded text-black-50 font-mono">${pdfDoc.numPages}</span>
            </div>
            <div id="pdf-thumbnails-list" class="flex-grow overflow-y-auto p-2 space-y-3 bg-black-900/10 no-scroll">
              <!-- Thumbnail items -->
            </div>
          </div>

          <!-- Main PDF Viewer Body -->
          <div class="flex-grow flex flex-col min-w-0 bg-[#121212] h-full relative">
            <!-- Inner PDF toolbar -->
            <div class="h-12 bg-black-900/40 border-b border-black-600/20 flex items-center justify-between px-4 z-20 flex-shrink-0 select-none">
              <!-- Search Panel -->
              <div class="flex items-center space-x-2">
                <div class="relative">
                  <input type="text" id="pdf-search-input" placeholder="Search page content..." class="bg-black-600 text-white text-[11px] px-2.5 py-1 pl-7 rounded border border-black-400 focus:outline-none focus:border-accent w-32 sm:w-44 placeholder-black-100">
                  <span class="material-symbols text-xs text-black-50 absolute left-2 top-2">search</span>
                </div>
                <button id="pdf-search-prev-btn" class="p-1 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors" title="Previous Match">
                  <span class="material-symbols text-sm">navigate_before</span>
                </button>
                <button id="pdf-search-next-btn" class="p-1 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors" title="Next Match">
                  <span class="material-symbols text-sm">navigate_next</span>
                </button>
                <span id="pdf-search-results-count" class="text-[10px] text-black-50 font-mono"></span>
              </div>

              <!-- Zoom and Page Navigation -->
              <div class="flex items-center space-x-3">
                <!-- Zoom Out -->
                <button id="pdf-zoom-out-btn" class="p-1 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors" title="Zoom Out">
                  <span class="material-symbols text-base">zoom_out</span>
                </button>
                <span id="pdf-zoom-level-label" class="text-[11px] text-white font-mono min-w-[32px] text-center">100%</span>
                <!-- Zoom In -->
                <button id="pdf-zoom-in-btn" class="p-1 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors" title="Zoom In">
                  <span class="material-symbols text-base">zoom_in</span>
                </button>

                <div class="h-4 w-px bg-black-600"></div>

                <!-- Page Jump -->
                <div class="flex items-center space-x-1">
                  <button id="pdf-prev-page-btn" class="p-1 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors" title="Previous Page">
                    <span class="material-symbols text-base">chevron_left</span>
                  </button>
                  <div class="flex items-center space-x-0.5">
                    <input type="number" id="pdf-current-page-input" value="1" min="1" max="${pdfDoc.numPages}" class="w-8 bg-black-600 border border-black-400 rounded py-0.5 text-center text-[11px] text-white font-mono focus:outline-none focus:border-accent [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none">
                    <span class="text-[11px] text-black-100 font-mono">/</span>
                    <span id="pdf-total-pages-label" class="text-[11px] text-black-100 font-mono">${pdfDoc.numPages}</span>
                  </div>
                  <button id="pdf-next-page-btn" class="p-1 hover:bg-black-500 rounded text-black-50 hover:text-white transition-colors" title="Next Page">
                    <span class="material-symbols text-base">chevron_right</span>
                  </button>
                </div>
              </div>
            </div>

            <!-- Main Render Area -->
            <div id="pdf-page-viewport" class="flex-grow overflow-auto p-4 flex items-start justify-center">
              <div class="relative bg-white shadow-xl rounded border border-black-500/20 max-w-full">
                <canvas id="pdf-canvas" class="block max-w-full"></canvas>
              </div>
            </div>
          </div>
        </div>
      `;

      let pdfCurrentPage = 1;
      let pdfZoomLevel = 1.0;
      let pdfRendering = false;
      let pdfPendingPage = null;

      const canvas = document.getElementById('pdf-canvas');
      const ctx = canvas.getContext('2d');

      const renderPdfPage = async (pageNum) => {
        if (pdfRendering) {
          pdfPendingPage = pageNum;
          return;
        }
        pdfRendering = true;
        pdfCurrentPage = pageNum;

        try {
          const page = await pdfDoc.getPage(pageNum);
          const viewport = page.getViewport({ scale: pdfZoomLevel });
          canvas.width = viewport.width;
          canvas.height = viewport.height;

          const renderContext = {
            canvasContext: ctx,
            viewport: viewport
          };

          await page.render(renderContext).promise;
          pdfRendering = false;

          // Update inputs
          const pageInput = document.getElementById('pdf-current-page-input');
          if (pageInput) pageInput.value = pageNum;

          // Highlight current thumbnail
          const items = document.querySelectorAll('.pdf-thumbnail-item');
          items.forEach(item => {
            const p = parseInt(item.getAttribute('data-page'), 10);
            if (p === pageNum) {
              item.classList.add('border-accent', 'bg-accent/10');
              item.classList.remove('border-black-400/30');
              item.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
            } else {
              item.classList.remove('border-accent', 'bg-accent/10');
              item.classList.add('border-black-400/30');
            }
          });

          if (pdfPendingPage !== null) {
            const next = pdfPendingPage;
            pdfPendingPage = null;
            renderPdfPage(next);
          }
        } catch (err) {
          console.error('Error rendering PDF page:', err);
          pdfRendering = false;
        }
      };

      // Populate thumbnails list
      const thumbList = document.getElementById('pdf-thumbnails-list');
      const observer = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
          if (entry.isIntersecting) {
            const thumbItem = entry.target;
            const pNum = parseInt(thumbItem.getAttribute('data-page'), 10);
            if (thumbItem.getAttribute('data-rendered') !== 'true') {
              thumbItem.setAttribute('data-rendered', 'true');
              renderThumbnail(pNum, thumbItem);
            }
          }
        });
      }, { root: thumbList, threshold: 0.1 });

      const renderThumbnail = async (pNum, itemDiv) => {
        try {
          const page = await pdfDoc.getPage(pNum);
          const thumbCanvas = document.createElement('canvas');
          thumbCanvas.className = 'w-full object-contain shadow rounded border border-black-400/20';
          const tCtx = thumbCanvas.getContext('2d');
          
          const viewport = page.getViewport({ scale: 0.15 });
          thumbCanvas.width = viewport.width;
          thumbCanvas.height = viewport.height;
          
          await page.render({ canvasContext: tCtx, viewport }).promise;
          
          itemDiv.innerHTML = '';
          itemDiv.appendChild(thumbCanvas);
          
          const pageLabel = document.createElement('div');
          pageLabel.className = 'text-[9px] text-black-50 font-mono mt-1 text-center';
          pageLabel.textContent = pNum;
          itemDiv.appendChild(pageLabel);
        } catch (e) {
          console.warn('Failed to render thumbnail for page', pNum, e);
        }
      };

      for (let i = 1; i <= pdfDoc.numPages; i++) {
        const itemDiv = document.createElement('div');
        itemDiv.className = 'pdf-thumbnail-item border border-black-400/30 rounded p-1 cursor-pointer flex flex-col items-center hover:border-accent transition-colors';
        itemDiv.setAttribute('data-page', i);
        itemDiv.innerHTML = `
          <div class="w-full aspect-[3/4] bg-black-600/30 flex items-center justify-center text-[10px] text-black-100 font-mono">
            Page ${i}
          </div>
        `;
        
        itemDiv.onclick = () => {
          renderPdfPage(i);
        };
        
        thumbList.appendChild(itemDiv);
        observer.observe(itemDiv);
      }

      // Event binds
      document.getElementById('pdf-prev-page-btn').onclick = () => {
        if (pdfCurrentPage > 1) {
          renderPdfPage(pdfCurrentPage - 1);
        }
      };
      
      document.getElementById('pdf-next-page-btn').onclick = () => {
        if (pdfCurrentPage < pdfDoc.numPages) {
          renderPdfPage(pdfCurrentPage + 1);
        }
      };

      document.getElementById('pdf-current-page-input').onchange = (e) => {
        let val = parseInt(e.target.value, 10);
        if (isNaN(val) || val < 1) val = 1;
        if (val > pdfDoc.numPages) val = pdfDoc.numPages;
        renderPdfPage(val);
      };

      document.getElementById('pdf-zoom-in-btn').onclick = () => {
        pdfZoomLevel = Math.min(pdfZoomLevel + 0.2, 3.0);
        document.getElementById('pdf-zoom-level-label').textContent = `${Math.round(pdfZoomLevel * 100)}%`;
        renderPdfPage(pdfCurrentPage);
      };

      document.getElementById('pdf-zoom-out-btn').onclick = () => {
        pdfZoomLevel = Math.max(pdfZoomLevel - 0.2, 0.5);
        document.getElementById('pdf-zoom-level-label').textContent = `${Math.round(pdfZoomLevel * 100)}%`;
        renderPdfPage(pdfCurrentPage);
      };

      // Search functionality
      let pdfSearchResults = [];
      let pdfSearchCurrentIndex = -1;

      const countLabel = document.getElementById('pdf-search-results-count');
      const searchInput = document.getElementById('pdf-search-input');

      const performSearch = async () => {
        const query = searchInput.value.trim();
        if (!query) {
          pdfSearchResults = [];
          pdfSearchCurrentIndex = -1;
          countLabel.textContent = '';
          return;
        }

        countLabel.textContent = 'Searching...';
        const matches = [];
        const lowerQuery = query.toLowerCase();

        for (let p = 1; p <= pdfDoc.numPages; p++) {
          try {
            const page = await pdfDoc.getPage(p);
            const textContent = await page.getTextContent();
            const textStr = textContent.items.map(item => item.str).join(' ');
            if (textStr.toLowerCase().includes(lowerQuery)) {
              matches.push(p);
            }
          } catch (e) {
            console.warn('Error reading page text:', p, e);
          }
        }

        pdfSearchResults = matches;
        pdfSearchCurrentIndex = matches.length > 0 ? 0 : -1;

        if (matches.length === 0) {
          countLabel.textContent = 'No matches';
        } else {
          jumpToSearchMatch();
        }
      };

      const jumpToSearchMatch = () => {
        if (pdfSearchCurrentIndex < 0 || pdfSearchCurrentIndex >= pdfSearchResults.length) return;
        countLabel.textContent = `${pdfSearchCurrentIndex + 1} of ${pdfSearchResults.length}`;
        renderPdfPage(pdfSearchResults[pdfSearchCurrentIndex]);
      };

      searchInput.onchange = performSearch;
      searchInput.onkeydown = (e) => {
        if (e.key === 'Enter') {
          performSearch();
        }
      };

      document.getElementById('pdf-search-prev-btn').onclick = () => {
        if (pdfSearchResults.length === 0) return;
        pdfSearchCurrentIndex = (pdfSearchCurrentIndex - 1 + pdfSearchResults.length) % pdfSearchResults.length;
        jumpToSearchMatch();
      };

      document.getElementById('pdf-search-next-btn').onclick = () => {
        if (pdfSearchResults.length === 0) return;
        pdfSearchCurrentIndex = (pdfSearchCurrentIndex + 1) % pdfSearchResults.length;
        jumpToSearchMatch();
      };

      // Render first page initial
      await renderPdfPage(1);

      if (spinner) spinner.remove();
    } catch (err) {
      console.error('Failed to load PDF.js viewer:', err);
      contentBody.innerHTML = `
        <div class="text-center text-red-500 py-8 text-sm">
          <p>Failed to load PDF viewer.</p>
          <p class="text-xs text-black-100 mt-1">${escapeHtml(err.message || err)}</p>
        </div>
      `;
    }
  } 
  else if (format === 'epub') {
    // Show EPUB-specific controls
    document.getElementById('epub-controls').classList.remove('hidden');
    document.getElementById('epub-prev-page-btn').classList.remove('hidden');
    document.getElementById('epub-next-page-btn').classList.remove('hidden');

    try {
      // Dynamic load JSZip and EpubJS
      await loadScript('https://cdn.jsdelivr.net/npm/jszip/dist/jszip.min.js');
      await loadScript('https://cdn.jsdelivr.net/npm/epubjs/dist/epub.min.js');

      // Fetch saved progress from server
      let savedLocation = null;
      try {
        const progressObj = await request('GET', `/api/me/progress/${itemId}`);
        if (progressObj) {
          if (progressObj.ebookLocation) {
            savedLocation = progressObj.ebookLocation;
          }
          if (progressObj.ebookProgress !== undefined && progressObj.ebookProgress !== null) {
            currentProgress = progressObj.ebookProgress;
          }
        }
      } catch (err) {
        console.log("No progress found for book", err);
      }

      // Create rendition container
      const viewer = document.createElement('div');
      viewer.id = 'epub-viewer';
      
      contentBody.innerHTML = '';
      contentBody.appendChild(viewer);

      // Immediately set the progress bar and progress percent text
      const progressPercentText = document.getElementById('reader-progress-percent');
      const progressBarFill = document.getElementById('reader-progress-bar-fill');
      const initPctDisplay = Math.round(currentProgress * 100);
      if (progressPercentText) progressPercentText.textContent = `Progress: ${initPctDisplay}%`;
      if (progressBarFill) progressBarFill.style.width = `${initPctDisplay}%`;

      // Initialize book & rendition
      const ebookUrl = resolvePath(`/api/items/${itemId}/ebook?token=${token}`);
      book = ePub(ebookUrl, { openAs: 'epub' });

      // Mouse wheel navigation handler
      let lastScrollTime = 0;
      const scrollThrottle = 350; // ms
      const handleWheel = (e) => {
        // Prevent default scrolling only in paginated mode
        if (currentFlow === 'paginated') {
          e.preventDefault();
        } else {
          return; // Let normal scrolling happen in scrolled flow
        }

        const now = Date.now();
        if (now - lastScrollTime < scrollThrottle) return;

        const absX = Math.abs(e.deltaX);
        const absY = Math.abs(e.deltaY);

        if (Math.max(absX, absY) < 10) return;

        if (absX > absY) {
          if (e.deltaX > 0) {
            rendition.next();
            lastScrollTime = now;
          } else if (e.deltaX < 0) {
            rendition.prev();
            lastScrollTime = now;
          }
        } else {
          if (e.deltaY > 0) {
            rendition.next();
            lastScrollTime = now;
          } else if (e.deltaY < 0) {
            rendition.prev();
            lastScrollTime = now;
          }
        }
      };

      const getThemeRules = (theme, font, lineHeight, margin) => {
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
      };

      const renderExistingHighlights = () => {
        if (!rendition) return;
        const curUser = window.currentUser || {};
        const bms = (curUser.bookmarks || []).filter(b => b.libraryItemId === itemId && b.cfi);
        
        // Remove existing annotations first to avoid duplicates
        bms.forEach(bm => {
          try {
            let hlClass = "epubjs-hl-yellow";
            if (bm.color === '#8bc34a') hlClass = "epubjs-hl-green";
            else if (bm.color === '#f48fb1') hlClass = "epubjs-hl-pink";
            else if (bm.color === '#29b6f6') hlClass = "epubjs-hl-blue";
            rendition.annotations.remove(bm.cfi, "highlight");
          } catch(e) {}
        });

        // Add back
        bms.forEach(bm => {
          try {
            let hlClass = "epubjs-hl-yellow";
            if (bm.color === '#8bc34a') hlClass = "epubjs-hl-green";
            else if (bm.color === '#f48fb1') hlClass = "epubjs-hl-pink";
            else if (bm.color === '#29b6f6') hlClass = "epubjs-hl-blue";
            
            rendition.annotations.add("highlight", bm.cfi, {}, () => {}, hlClass);
          } catch (e) {
            console.warn("Failed to render highlight:", bm.cfi, e);
          }
        });
      };

      const initRendition = async (targetLocation) => {
        if (rendition) {
          try {
            rendition.destroy();
          } catch(e) {}
        }
        
        if (currentFlow === 'scrolled-doc') {
          viewer.className = 'w-full h-full max-w-4xl bg-white shadow-2xl rounded-md transition-colors duration-300 overflow-y-auto';
        } else {
          viewer.className = 'w-full h-full max-w-4xl bg-white shadow-2xl rounded-md transition-colors duration-300 overflow-hidden';
        }
        
        viewer.innerHTML = '';

        rendition = book.renderTo(viewer, {
          width: "100%",
          height: "100%",
          flow: currentFlow,
          manager: currentFlow === 'scrolled-doc' ? 'continuous' : 'default',
          spread: currentFlow === 'scrolled-doc' ? 'none' : currentLayout
        });

        // Register content hook for click, keyup, scroll and font injection inside iframe
        rendition.hooks.content.register((contents) => {
          // Inject custom fonts
          const linkDyslexic = contents.document.createElement('link');
          linkDyslexic.rel = 'stylesheet';
          linkDyslexic.href = 'https://cdn.jsdelivr.net/npm/open-dyslexic@1.0.3/open-dyslexic.css';
          contents.document.head.appendChild(linkDyslexic);
          
          const linkInter = contents.document.createElement('link');
          linkInter.rel = 'stylesheet';
          linkInter.href = 'https://fonts.googleapis.com/css2?family=Inter:wght@400;600&display=swap';
          contents.document.head.appendChild(linkInter);

          const linkMerriweather = contents.document.createElement('link');
          linkMerriweather.rel = 'stylesheet';
          linkMerriweather.href = 'https://fonts.googleapis.com/css2?family=Merriweather:wght@400;700&display=swap';
          contents.document.head.appendChild(linkMerriweather);

          // Highlight Stylesheet inside iframe
          const style = contents.document.createElement('style');
          style.innerHTML = `
            ::selection { background-color: rgba(255, 235, 59, 0.45) !important; }
            .epubjs-hl-yellow { background-color: rgba(255, 235, 59, 0.45) !important; cursor: pointer; }
            .epubjs-hl-green { background-color: rgba(139, 195, 74, 0.45) !important; cursor: pointer; }
            .epubjs-hl-pink { background-color: rgba(244, 143, 177, 0.45) !important; cursor: pointer; }
            .epubjs-hl-blue { background-color: rgba(41, 182, 246, 0.45) !important; cursor: pointer; }
          `;
          contents.document.head.appendChild(style);

          contents.document.addEventListener("click", () => {
            closeTOCDrawer();
            const popover = document.getElementById('reader-settings-popover');
            if (popover) popover.classList.add('hidden');
          });
          contents.document.addEventListener("keyup", keyListener);
          contents.document.addEventListener("wheel", handleWheel, { passive: false });
        });

        // Relocated event
        rendition.on("relocated", (location) => {
          const cfi = location.start.cfi;
          
          let pct = currentProgress;
          if (book.locations && book.locations.total > 0) {
            pct = book.locations.percentageFromCfi(cfi);
            currentProgress = pct;
          }
          
          const progressPercentText = document.getElementById('reader-progress-percent');
          const progressBarFill = document.getElementById('reader-progress-bar-fill');
          const pctDisplay = Math.round(pct * 100);
          
          if (progressPercentText) progressPercentText.textContent = `Progress: ${pctDisplay}%`;
          if (progressBarFill) progressBarFill.style.width = `${pctDisplay}%`;
          
          const pageInfo = document.getElementById('epub-page-info');
          if (pageInfo) {
            if (location.start.displayed && location.start.displayed.page && location.start.displayed.total) {
              pageInfo.textContent = `Page ${location.start.displayed.page} of ${location.start.displayed.total}`;
            } else {
              pageInfo.textContent = '';
            }
          }
          
          queueSaveProgress(cfi, pct);
          
          // Render annotations/highlights
          renderExistingHighlights();
          
          // Clear TTS sentences when relocation happens
          if (isTtsPlaying) {
            pauseTts();
          }
          ttsSentences = [];
          currentSentenceIdx = 0;
        });

        // Selected event for highlight creation
        rendition.on("selected", (cfiRange, contents) => {
          const selection = contents.window.getSelection();
          const text = selection.toString().trim();
          if (!text) return;
          showHighlightModal(cfiRange, text);
        });

        await rendition.display(targetLocation || undefined);
        applyTypography();
      };

      const applyTypography = () => {
        if (!rendition) return;
        
        rendition.themes.register("light", getThemeRules("light", currentFont, currentLineHeight, currentMargin));
        rendition.themes.register("sepia", getThemeRules("sepia", currentFont, currentLineHeight, currentMargin));
        rendition.themes.register("warm", getThemeRules("warm", currentFont, currentLineHeight, currentMargin));
        rendition.themes.register("dark", getThemeRules("dark", currentFont, currentLineHeight, currentMargin));
        
        rendition.themes.select(currentTheme);
        
        if (currentFlow === 'paginated') {
          rendition.spread(currentLayout);
        }
        rendition.resize();
        
        updateSettingsUIActiveStates();
        saveSettings();
      };

      const updateSettingsUIActiveStates = () => {
        const fontSelect = document.getElementById('reader-font-family-select');
        if (fontSelect) fontSelect.value = currentFont;
        
        document.querySelectorAll('.reader-line-spacing-btn').forEach(btn => {
          const val = btn.getAttribute('data-val');
          if (val === currentLineHeight) {
            btn.className = `reader-line-spacing-btn text-xs py-1 text-center rounded transition-colors ${activeBtnClass}`;
          } else {
            btn.className = `reader-line-spacing-btn text-xs py-1 text-center rounded transition-colors ${inactiveBtnClass}`;
          }
        });
        
        document.querySelectorAll('.reader-margin-btn').forEach(btn => {
          const val = btn.getAttribute('data-val');
          if (val === currentMargin) {
            btn.className = `reader-margin-btn text-xs py-1 text-center rounded transition-colors ${activeBtnClass}`;
          } else {
            btn.className = `reader-margin-btn text-xs py-1 text-center rounded transition-colors ${inactiveBtnClass}`;
          }
        });

        const flowPaginatedBtn = document.getElementById('reader-flow-paginated-btn');
        const flowScrolledBtn = document.getElementById('reader-flow-scrolled-btn');
        if (flowPaginatedBtn && flowScrolledBtn) {
          if (currentFlow === 'paginated') {
            flowPaginatedBtn.className = `text-xs py-1 text-center rounded transition-colors ${activeBtnClass}`;
            flowScrolledBtn.className = `text-xs py-1 text-center rounded transition-colors ${inactiveBtnClass}`;
          } else {
            flowPaginatedBtn.className = `text-xs py-1 text-center rounded transition-colors ${inactiveBtnClass}`;
            flowScrolledBtn.className = `text-xs py-1 text-center rounded transition-colors ${activeBtnClass}`;
          }
        }
        
        const layoutSingleBtn = document.getElementById('reader-layout-single-btn');
        const layoutSpreadBtn = document.getElementById('reader-layout-spread-btn');
        const layoutSection = document.getElementById('reader-page-layout-section');
        
        if (currentFlow === 'scrolled-doc') {
          if (layoutSection) layoutSection.classList.add('hidden');
        } else {
          if (layoutSection) layoutSection.classList.remove('hidden');
          if (layoutSingleBtn && layoutSpreadBtn) {
            if (currentLayout === 'none') {
              layoutSingleBtn.className = `text-xs py-1 text-center rounded transition-colors ${activeBtnClass}`;
              layoutSpreadBtn.className = `text-xs py-1 text-center rounded transition-colors ${inactiveBtnClass}`;
            } else {
              layoutSingleBtn.className = `text-xs py-1 text-center rounded transition-colors ${inactiveBtnClass}`;
              layoutSpreadBtn.className = `text-xs py-1 text-center rounded transition-colors ${activeBtnClass}`;
            }
          }
        }
      };

      const changeFlowOrLayout = async (newFlow, newLayout) => {
        const currentCfi = rendition ? rendition.location.start.cfi : (savedLocation || undefined);
        currentFlow = newFlow;
        currentLayout = newLayout;
        saveSettings();
        await initRendition(currentCfi);
      };

      // Throttled progress save
      const queueSaveProgress = (cfi, progressPercent) => {
        if (progressSaveTimeout) clearTimeout(progressSaveTimeout);
        progressSaveTimeout = setTimeout(async () => {
          try {
            await request('PATCH', `/api/me/progress/${itemId}`, {
              ebookLocation: cfi,
              ebookProgress: progressPercent
            });
          } catch (err) {
            console.error("Failed to save progress:", err);
          }
        }, 3000);
      };

      // First initial rendition load
      await initRendition(savedLocation || undefined);

      const applyTheme = (theme) => {
        currentTheme = theme;
        saveSettings();
        
        const vContainer = document.getElementById('epub-viewer');
        const cBody = document.getElementById('reader-content-body');
        
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
          // dark
          if (vContainer) {
            vContainer.style.backgroundColor = '#1a1a1a';
            vContainer.style.color = '#e0e0e0';
          }
          if (cBody) cBody.style.backgroundColor = '#121212';
        }
        
        if (rendition) {
          rendition.themes.register("light", getThemeRules("light", currentFont, currentLineHeight, currentMargin));
          rendition.themes.register("sepia", getThemeRules("sepia", currentFont, currentLineHeight, currentMargin));
          rendition.themes.register("warm", getThemeRules("warm", currentFont, currentLineHeight, currentMargin));
          rendition.themes.register("dark", getThemeRules("dark", currentFont, currentLineHeight, currentMargin));
          rendition.themes.select(theme);
        }
        
        document.querySelectorAll('[data-theme]').forEach(btn => {
          if (btn.getAttribute('data-theme') === theme) {
            btn.className = getThemeButtonActiveStyle(theme);
          } else {
            btn.className = getThemeButtonInactiveStyle(btn.getAttribute('data-theme'));
          }
        });
      };

      // Set initial theme
      applyTheme(currentTheme);

      // Theme button click events
      document.querySelectorAll('[data-theme]').forEach(btn => {
        btn.onclick = (e) => {
          applyTheme(e.target.getAttribute('data-theme'));
        };
      });

      // Settings popover toggle
      const settingsBtn = document.getElementById('reader-settings-btn');
      const settingsPopover = document.getElementById('reader-settings-popover');
      if (settingsBtn && settingsPopover) {
        settingsBtn.onclick = (e) => {
          e.stopPropagation();
          settingsPopover.classList.toggle('hidden');
        };
      }

      // Close settings when clicking outside
      const clickOutsideSettings = (e) => {
        if (settingsPopover && !settingsPopover.contains(e.target) && e.target !== settingsBtn && !settingsBtn.contains(e.target)) {
          settingsPopover.classList.add('hidden');
        }
      };
      document.addEventListener('click', clickOutsideSettings);

      // Initialize popover UI active states
      updateSettingsUIActiveStates();

      // Font Family Select change event
      const fontSelect = document.getElementById('reader-font-family-select');
      if (fontSelect) {
        fontSelect.onchange = (e) => {
          currentFont = e.target.value;
          applyTypography();
        };
      }

      // Line Spacing Button click events
      document.querySelectorAll('.reader-line-spacing-btn').forEach(btn => {
        btn.onclick = (e) => {
          currentLineHeight = e.target.getAttribute('data-val');
          applyTypography();
        };
      });

      // Margin Button click events
      document.querySelectorAll('.reader-margin-btn').forEach(btn => {
        btn.onclick = (e) => {
          currentMargin = e.target.getAttribute('data-val');
          applyTypography();
        };
      });

      // Flow Layout choice Button click events
      const flowPaginatedBtn = document.getElementById('reader-flow-paginated-btn');
      if (flowPaginatedBtn) {
        flowPaginatedBtn.onclick = () => {
          changeFlowOrLayout('paginated', currentLayout);
        };
      }
      const flowScrolledBtn = document.getElementById('reader-flow-scrolled-btn');
      if (flowScrolledBtn) {
        flowScrolledBtn.onclick = () => {
          changeFlowOrLayout('scrolled-doc', 'none');
        };
      }

      // Layout Button click events
      const layoutSingleBtn = document.getElementById('reader-layout-single-btn');
      if (layoutSingleBtn) {
        layoutSingleBtn.onclick = () => {
          changeFlowOrLayout('paginated', 'none');
        };
      }
      const layoutSpreadBtn = document.getElementById('reader-layout-spread-btn');
      if (layoutSpreadBtn) {
        layoutSpreadBtn.onclick = () => {
          changeFlowOrLayout('paginated', 'auto');
        };
      }

      // Font size button clicks
      document.getElementById('reader-font-dec-btn').onclick = () => {
        if (currentFontSize > 60) {
          currentFontSize -= 10;
          rendition.themes.fontSize(`${currentFontSize}%`);
          saveSettings();
        }
      };
      document.getElementById('reader-font-inc-btn').onclick = () => {
        if (currentFontSize < 200) {
          currentFontSize += 10;
          rendition.themes.fontSize(`${currentFontSize}%`);
          saveSettings();
        }
      };

      // Highlight/Bookmark logic
      const showHighlightModal = (cfiRange, text) => {
        selectedCfiRange = cfiRange;
        selectedTextStr = text;
        
        const modal = document.getElementById('reader-highlight-modal');
        const textBox = document.getElementById('highlight-selected-text');
        const noteInput = document.getElementById('highlight-note-input');
        
        if (textBox) textBox.textContent = text;
        if (noteInput) noteInput.value = "";
        
        document.querySelectorAll('.hl-color-btn').forEach(btn => {
          if (btn.getAttribute('data-color') === activeColor) {
            btn.classList.add('ring-2', 'ring-accent', 'ring-offset-2', 'ring-offset-primary');
          } else {
            btn.classList.remove('ring-2', 'ring-accent', 'ring-offset-2', 'ring-offset-primary');
          }
        });

        if (modal) modal.classList.remove('hidden');
      };

      document.querySelectorAll('.hl-color-btn').forEach(btn => {
        btn.onclick = (e) => {
          activeColor = e.target.getAttribute('data-color');
          document.querySelectorAll('.hl-color-btn').forEach(b => {
            if (b.getAttribute('data-color') === activeColor) {
              b.classList.add('ring-2', 'ring-accent', 'ring-offset-2', 'ring-offset-primary');
            } else {
              b.classList.remove('ring-2', 'ring-accent', 'ring-offset-2', 'ring-offset-primary');
            }
          });
        };
      });

      document.getElementById('highlight-save-btn').onclick = async () => {
        const noteInput = document.getElementById('highlight-note-input');
        const noteText = noteInput ? noteInput.value.trim() : "";
        
        try {
          const resp = await request('POST', `/api/me/item/${itemId}/bookmark`, {
            time: Date.now() / 1000,
            title: selectedTextStr,
            note: noteText,
            color: activeColor,
            cfi: selectedCfiRange
          });
          
          document.getElementById('reader-highlight-modal').classList.add('hidden');
          
          // Deselect text in viewer iframe
          if (rendition) {
            try {
              const contents = rendition.getContents();
              if (contents && contents.length > 0) {
                contents[0].window.getSelection().removeAllRanges();
              }
            } catch (e) {}
          }

          // Trigger rendering highlights and refresh side drawer
          renderExistingHighlights();
          await refreshBookmarksTab();
          
        } catch (err) {
          console.error("Failed to save highlight:", err);
          alert("Failed to save highlight");
        }
      };

      document.getElementById('highlight-cancel-btn').onclick = () => {
        document.getElementById('reader-highlight-modal').classList.add('hidden');
        if (rendition) {
          try {
            const contents = rendition.getContents();
            if (contents && contents.length > 0) {
              contents[0].window.getSelection().removeAllRanges();
            }
          } catch (e) {}
        }
      };

      // Drawer Tab Swapping
      const tabChapters = document.getElementById('drawer-tab-chapters');
      const tabBookmarks = document.getElementById('drawer-tab-bookmarks');
      const listChapters = document.getElementById('reader-toc-list');
      const listBookmarks = document.getElementById('reader-bookmarks-list');

      if (tabChapters && tabBookmarks && listChapters && listBookmarks) {
        tabChapters.onclick = () => {
          tabChapters.className = `flex-1 text-center py-1 text-xs rounded transition-colors ${activeBtnClass}`;
          tabBookmarks.className = `flex-1 text-center py-1 text-xs rounded transition-colors ${inactiveBtnClass}`;
          listChapters.classList.remove('hidden');
          listBookmarks.classList.add('hidden');
        };
        tabBookmarks.onclick = () => {
          tabBookmarks.className = `flex-1 text-center py-1 text-xs rounded transition-colors ${activeBtnClass}`;
          tabChapters.className = `flex-1 text-center py-1 text-xs rounded transition-colors ${inactiveBtnClass}`;
          listChapters.classList.add('hidden');
          listBookmarks.classList.remove('hidden');
          refreshBookmarksTab();
        };
      }

      // TTS implementation
      const ttsBtn = document.getElementById('reader-tts-btn');
      const ttsPopover = document.getElementById('reader-tts-popover');
      
      const populateVoices = () => {
        const select = document.getElementById('tts-voice-select');
        if (!select) return;
        
        const voices = window.speechSynthesis.getVoices();
        select.innerHTML = '';
        
        voices.forEach(voice => {
          const opt = document.createElement('option');
          opt.value = voice.name;
          opt.textContent = `${voice.name} (${voice.lang})`;
          if (voice.default || voice.lang.startsWith('en')) {
            opt.selected = true;
            if (!selectedVoiceName) selectedVoiceName = voice.name;
          }
          select.appendChild(opt);
        });
      };

      if (ttsBtn && ttsPopover) {
        ttsBtn.onclick = (e) => {
          e.stopPropagation();
          ttsPopover.classList.toggle('hidden');
          if (!ttsPopover.classList.contains('hidden')) {
            populateVoices();
          }
        };
      }

      const closeTtsBtn = document.getElementById('close-tts-btn');
      if (closeTtsBtn) {
        closeTtsBtn.onclick = () => {
          ttsPopover.classList.add('hidden');
        };
      }

      const clickOutsideTts = (e) => {
        if (ttsPopover && !ttsPopover.contains(e.target) && e.target !== ttsBtn && !ttsBtn.contains(e.target)) {
          ttsPopover.classList.add('hidden');
        }
      };
      document.addEventListener('click', clickOutsideTts);

      if ('speechSynthesis' in window) {
        if (window.speechSynthesis.onvoiceschanged !== undefined) {
          window.speechSynthesis.onvoiceschanged = populateVoices;
        }
        populateVoices();
      }

      const getCleanSentences = () => {
        if (!rendition) return [];
        const contents = rendition.getContents();
        if (!contents || contents.length === 0) return [];
        const bodyText = contents[0].document.body.innerText || "";
        const raw = bodyText.split(/(?<=[.!?])\s+/);
        return raw.map(s => s.trim()).filter(s => s.length > 2);
      };

      const speakCurrentSentence = () => {
        if (!('speechSynthesis' in window)) {
          alert("Text-to-speech is not supported in this browser.");
          return;
        }
        
        window.speechSynthesis.cancel();
        
        if (currentSentenceIdx < 0 || currentSentenceIdx >= ttsSentences.length) {
          if (rendition) {
            rendition.next().then(() => {
              setTimeout(() => {
                ttsSentences = getCleanSentences();
                currentSentenceIdx = 0;
                if (isTtsPlaying) {
                  speakCurrentSentence();
                }
              }, 600);
            });
          }
          return;
        }

        const sentence = ttsSentences[currentSentenceIdx];
        ttsUtterance = new SpeechSynthesisUtterance(sentence);
        ttsUtterance.rate = ttsRate;
        
        if (selectedVoiceName) {
          const voices = window.speechSynthesis.getVoices();
          const voice = voices.find(v => v.name === selectedVoiceName);
          if (voice) ttsUtterance.voice = voice;
        }

        ttsUtterance.onend = () => {
          if (isTtsPlaying) {
            currentSentenceIdx++;
            speakCurrentSentence();
          }
        };

        ttsUtterance.onerror = (e) => {
          console.error("SpeechSynthesis error:", e);
          isTtsPlaying = false;
          updateTtsUI();
        };

        window.speechSynthesis.speak(ttsUtterance);
      };

      const updateTtsUI = () => {
        const playIcon = document.getElementById('tts-play-icon');
        if (playIcon) {
          playIcon.textContent = isTtsPlaying ? 'pause' : 'play_arrow';
        }
      };

      const playTts = () => {
        if (ttsSentences.length === 0) {
          ttsSentences = getCleanSentences();
          currentSentenceIdx = 0;
        }
        
        if (ttsSentences.length === 0) {
          alert("No readable text found on this page.");
          return;
        }

        isTtsPlaying = true;
        updateTtsUI();
        speakCurrentSentence();
      };

      const pauseTts = () => {
        isTtsPlaying = false;
        updateTtsUI();
        window.speechSynthesis.cancel();
      };

      const stopTts = () => {
        isTtsPlaying = false;
        updateTtsUI();
        window.speechSynthesis.cancel();
        ttsSentences = [];
        currentSentenceIdx = 0;
      };

      const speedSlider = document.getElementById('tts-speed-slider');
      const speedVal = document.getElementById('tts-speed-val');
      if (speedSlider && speedVal) {
        speedSlider.oninput = (e) => {
          ttsRate = parseFloat(e.target.value);
          speedVal.textContent = `${ttsRate.toFixed(1)}x`;
          if (isTtsPlaying) {
            speakCurrentSentence();
          }
        };
      }

      const voiceSelect = document.getElementById('tts-voice-select');
      if (voiceSelect) {
        voiceSelect.onchange = (e) => {
          selectedVoiceName = e.target.value;
          if (isTtsPlaying) {
            speakCurrentSentence();
          }
        };
      }

      const playBtn = document.getElementById('tts-play-btn');
      if (playBtn) {
        playBtn.onclick = () => {
          if (isTtsPlaying) {
            pauseTts();
          } else {
            playTts();
          }
        };
      }

      const stopBtn = document.getElementById('tts-stop-btn');
      if (stopBtn) {
        stopBtn.onclick = () => {
          stopTts();
        };
      }

      const prevSentenceBtn = document.getElementById('tts-prev-sentence-btn');
      if (prevSentenceBtn) {
        prevSentenceBtn.onclick = () => {
          if (currentSentenceIdx > 0) {
            currentSentenceIdx--;
            if (isTtsPlaying) speakCurrentSentence();
          }
        };
      }
      const nextSentenceBtn = document.getElementById('tts-next-sentence-btn');
      if (nextSentenceBtn) {
        nextSentenceBtn.onclick = () => {
          if (currentSentenceIdx < ttsSentences.length - 1) {
            currentSentenceIdx++;
            if (isTtsPlaying) speakCurrentSentence();
          }
        };
      }

      // Event listeners
      document.addEventListener('keyup', keyListener);
      window.addEventListener('resize', handleResize);
      document.addEventListener('click', clickOutsideTOC);

      // Bookmarks view refresh helper
      const refreshBookmarksTab = async () => {
        try {
          window.currentUser = await request('GET', '/api/me');
        } catch (e) {
          console.warn("Failed to sync bookmarks:", e);
        }
        
        const curUser = window.currentUser || {};
        const bms = (curUser.bookmarks || []).filter(b => b.libraryItemId === itemId);
        
        bms.sort((a, b) => b.createdAt - a.createdAt);
        
        const container = document.getElementById('reader-bookmarks-list');
        if (!container) return;
        
        const searchVal = document.getElementById('bookmarks-search-input')?.value || "";
        
        const filteredBms = bms.filter(b => {
          if (!searchVal) return true;
          const q = searchVal.toLowerCase();
          return (b.title || "").toLowerCase().includes(q) || (b.note || "").toLowerCase().includes(q);
        });

        container.innerHTML = `
          <div class="px-2 pb-2">
            <input id="bookmarks-search-input" type="text" placeholder="Search highlights & notes..." class="w-full bg-black-600 border border-black-400 text-white rounded text-xs px-2 py-1.5 focus:outline-none focus:border-accent" value="${escapeHtml(searchVal)}" />
          </div>
          <div class="space-y-2 p-1 max-h-[calc(100vh-12rem)] overflow-y-auto no-scroll" id="bookmarks-items-container">
            ${filteredBms.length === 0 ? `
              <p class="text-xs text-black-100 italic text-center py-4">No highlights or notes found.</p>
            ` : filteredBms.map((b, idx) => {
              const isHighlight = !!b.cfi;
              const hlColor = b.color || '#ffeb3b';
              const borderStyle = isHighlight ? `border-left: 4px solid ${hlColor}; padding-left: 8px;` : '';
              
              return `
                <div class="bg-black-600/50 hover:bg-black-500/50 border border-black-400/30 rounded p-2.5 transition-colors relative group cursor-pointer" data-idx="${idx}" style="${borderStyle}">
                  <div class="flex justify-between items-start space-x-2">
                    <div class="flex-grow min-w-0 pr-4">
                      ${b.title ? `<p class="text-xs font-semibold text-white/90 line-clamp-3 italic">${escapeHtml(b.title)}</p>` : ''}
                      ${b.note ? `<p class="text-xs text-black-50 mt-1.5">${escapeHtml(b.note)}</p>` : ''}
                      <span class="text-[10px] text-black-100 block mt-2">${new Date(b.createdAt || b.time * 1000).toLocaleString()}</span>
                    </div>
                    <button class="delete-bookmark-btn text-black-100 hover:text-error transition-colors p-1 rounded hover:bg-black-400 focus:outline-none" data-time="${b.time}" title="Delete highlight">
                      <span class="material-symbols text-sm">delete</span>
                    </button>
                  </div>
                </div>
              `;
            }).join('')}
          </div>
        `;

        const searchInput = document.getElementById('bookmarks-search-input');
        if (searchInput) {
          searchInput.oninput = () => {
            refreshBookmarksTab();
          };
        }

        container.querySelectorAll('[data-idx]').forEach(el => {
          el.onclick = (e) => {
            if (e.target.closest('.delete-bookmark-btn')) return;
            const idx = parseInt(el.getAttribute('data-idx'));
            const bm = filteredBms[idx];
            if (bm && rendition) {
              if (bm.cfi) {
                rendition.display(bm.cfi);
              }
              closeTOCDrawer();
            }
          };
        });

        container.querySelectorAll('.delete-bookmark-btn').forEach(btn => {
          btn.onclick = async (e) => {
            e.stopPropagation();
            const timeVal = parseFloat(btn.getAttribute('data-time'));
            
            if (confirm("Are you sure you want to delete this highlight?")) {
              try {
                await request('DELETE', `/api/me/item/${itemId}/bookmark/${timeVal}`);
                
                const bm = bms.find(b => b.time === timeVal);
                if (bm && bm.cfi && rendition) {
                  let hlClass = "epubjs-hl-yellow";
                  if (bm.color === '#8bc34a') hlClass = "epubjs-hl-green";
                  else if (bm.color === '#f48fb1') hlClass = "epubjs-hl-pink";
                  else if (bm.color === '#29b6f6') hlClass = "epubjs-hl-blue";
                  rendition.annotations.remove(bm.cfi, hlClass);
                }

                await refreshBookmarksTab();
              } catch (err) {
                console.error("Failed to delete bookmark:", err);
                alert("Failed to delete bookmark");
              }
            }
          };
        });
      };

      // Navigation arrows click events
      document.getElementById('epub-prev-page-btn').onclick = () => rendition.prev();
      document.getElementById('epub-next-page-btn').onclick = () => rendition.next();

      // Navigation / TOC building
      book.loaded.navigation.then(nav => {
        const tocList = document.getElementById('reader-toc-list');
        tocList.innerHTML = '';
        
        const renderChapters = (chapters) => {
          chapters.forEach(chapter => {
            const btn = document.createElement('button');
            btn.className = 'w-full text-left px-3 py-2 text-xs rounded text-black-50 hover:text-white hover:bg-black-500 transition-colors truncate focus:outline-none';
            btn.textContent = chapter.label;
            btn.onclick = () => {
              rendition.display(chapter.href);
              closeTOCDrawer();
            };
            tocList.appendChild(btn);
            
            if (chapter.subitems && chapter.subitems.length > 0) {
              const subContainer = document.createElement('div');
              subContainer.className = 'pl-4 border-l border-black-600/30';
              chapter.subitems.forEach(sub => {
                const subBtn = document.createElement('button');
                subBtn.className = 'w-full text-left px-3 py-1.5 text-xs rounded text-black-100 hover:text-white hover:bg-black-500 transition-colors truncate focus:outline-none';
                subBtn.textContent = sub.label;
                subBtn.onclick = () => {
                  rendition.display(sub.href);
                  closeTOCDrawer();
                };
                subContainer.appendChild(subBtn);
              });
              tocList.appendChild(subContainer);
            }
          });
        };
        
        renderChapters(nav);
      });

      // Background locations generation
      book.ready.then(() => {
        return book.locations.generate(1024);
      }).then(() => {
        if (rendition.location) {
          const cfi = rendition.location.start.cfi;
          const pct = book.locations.percentageFromCfi(cfi);
          currentProgress = pct;
          const pctDisplay = Math.round(pct * 100);
          const progressPercentText = document.getElementById('reader-progress-percent');
          const progressBarFill = document.getElementById('reader-progress-bar-fill');
          if (progressPercentText) progressPercentText.textContent = `Progress: ${pctDisplay}%`;
          if (progressBarFill) progressBarFill.style.width = `${pctDisplay}%`;
        }
      });

    } catch (err) {
      console.error("Failed to initialize EPUB reader:", err);
      contentBody.innerHTML = `
        <div class="max-w-md bg-primary border border-black-300 rounded-md p-6 text-center space-y-4 shadow-2xl">
          <span class="material-symbols text-5xl text-error">error</span>
          <h4 class="text-lg font-bold text-white">EPUB Loading Error</h4>
          <p class="text-sm text-black-50 leading-relaxed">
            Failed to parse or render the EPUB ebook files. Error details: ${err.message || err}
          </p>
          <div class="flex justify-center pt-2">
            <button id="reader-error-close-btn" class="bg-black-400 hover:bg-black-300 text-white font-semibold px-4 py-2 rounded text-xs transition-colors">Close</button>
          </div>
        </div>
      `;
      document.getElementById('reader-error-close-btn').onclick = closeReader;
    }
  } 
  else {
    // Unsupported format fallback
    document.getElementById('epub-controls').classList.add('hidden');
    document.getElementById('reader-footer').classList.add('hidden');

    contentBody.innerHTML = `
      <div class="max-w-md bg-primary border border-black-300 rounded-md p-6 text-center space-y-4 shadow-2xl">
        <span class="material-symbols text-5xl text-accent">menu_book</span>
        <h4 class="text-lg font-bold text-white">${format.toUpperCase() || 'E-BOOK'} Reader Not Supported</h4>
        <p class="text-sm text-black-50 leading-relaxed">
          The built-in web reader currently supports EPUB and PDF formats. Would you like to download the file to read it on your device?
        </p>
        <div class="flex justify-center space-x-3 pt-2">
          <button id="reader-fallback-close-btn" class="bg-black-400 hover:bg-black-300 text-white font-semibold px-4 py-2 rounded text-xs transition-colors">Close</button>
          <button id="reader-fallback-download-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity shadow flex items-center space-x-1">
            <span class="material-symbols text-sm font-bold">download</span>
            <span>Download File</span>
          </button>
        </div>
      </div>
    `;

    document.getElementById('reader-fallback-close-btn').onclick = closeReader;
    document.getElementById('reader-fallback-download-btn').onclick = () => {
      window.open(resolvePath(`/api/items/${itemId}/ebook?token=${token}`), '_blank');
      closeReader();
    };
  }
}
