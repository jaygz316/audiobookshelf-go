// frontend/js/settings.js (Proposed Implementation)
import { request, resolvePath } from './api.js';
import { getActiveLibraryId, getLibrariesList, initLibrary } from './library.js';
import { onEvent, offEvent, sendEvent } from './socket.js';
import { logout } from './auth.js';

let currentSessions = [];
let selectedUserIdFilter = '';

function getFilteredSessions() {
  if (!selectedUserIdFilter) return currentSessions;
  return currentSessions.filter(s => s.userId === selectedUserIdFilter);
}

// Register socket listeners for real-time playback session updates
onEvent('playback_session_added', (session) => {
  console.log('[Socket] playback_session_added:', session);
  const idx = currentSessions.findIndex(s => s.id === session.id);
  if (idx === -1) {
    currentSessions.unshift(session);
  } else {
    currentSessions[idx] = session;
  }
  const tbody = document.getElementById('sessions-list-rows');
  if (tbody) {
    renderListeningSessionsListRows(getFilteredSessions());
  }
});

onEvent('playback_session_updated', (session) => {
  console.log('[Socket] playback_session_updated:', session);
  const idx = currentSessions.findIndex(s => s.id === session.id);
  if (idx !== -1) {
    currentSessions[idx] = session;
  } else {
    currentSessions.unshift(session);
  }
  const tbody = document.getElementById('sessions-list-rows');
  if (tbody) {
    renderListeningSessionsListRows(getFilteredSessions());
  }
});

onEvent('playback_session_removed', (data) => {
  console.log('[Socket] playback_session_removed:', data);
  currentSessions = currentSessions.filter(s => s.id !== data.id);
  const tbody = document.getElementById('sessions-list-rows');
  if (tbody) {
    renderListeningSessionsListRows(getFilteredSessions());
  }
});

export async function loadSettings() {
  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  const container = document.getElementById('bookshelf');
  if (!container) return;

  // Set the toolbar view title
  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Settings & Administration';
  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = 'System Config';

  // Render settings structure with sidebar layout
  container.innerHTML = `
    <div class="max-w-7xl mx-auto p-4 flex flex-col md:flex-row gap-6 h-full min-h-0">
      <!-- Left Settings Navigation Sidebar -->
      <div class="w-full md:w-64 flex-shrink-0 bg-primary/45 border border-black-400/40 rounded-lg p-2 flex flex-col space-y-1 h-fit" id="settings-tabs">
        <div class="text-xs font-semibold text-accent uppercase tracking-wider px-3 py-2 border-b border-black-400/40 mb-2">Settings</div>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-accent bg-black-500/80 flex items-center space-x-2" data-tab="users">
          <span class="material-symbols text-lg">group</span>
          <span>Users</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="libraries">
          <span class="material-symbols text-lg">local_library</span>
          <span>Libraries</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="server">
          <span class="material-symbols text-lg">dns</span>
          <span>Server Settings</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="auth">
          <span class="material-symbols text-lg">security</span>
          <span>Authentication</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="backups">
          <span class="material-symbols text-lg">backup</span>
          <span>Backups</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="providers">
          <span class="material-symbols text-lg">api</span>
          <span>Metadata Providers</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="upload">
          <span class="material-symbols text-lg">upload</span>
          <span>Upload Media</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="apikeys">
          <span class="material-symbols text-lg">vpn_key</span>
          <span>API Keys</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="listening-sessions">
          <span class="material-symbols text-lg">insights</span>
          <span>Listening Sessions</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="login-sessions">
          <span class="material-symbols text-lg">devices</span>
          <span>Login Sessions</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="logs">
          <span class="material-symbols text-lg">description</span>
          <span>Logs</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="notifications">
          <span class="material-symbols text-lg">notifications</span>
          <span>Notifications</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="feeds">
          <span class="material-symbols text-lg">rss_feed</span>
          <span>RSS Feeds</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="emails">
          <span class="material-symbols text-lg">mail</span>
          <span>E-Reader Email</span>
        </button>
        <button class="w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2" data-tab="shares">
          <span class="material-symbols text-lg">share</span>
          <span>Public Shares</span>
        </button>
      </div>

      <!-- Right Content Column -->
      <div class="flex-grow bg-primary/20 border border-black-400/20 rounded-lg p-6 min-w-0" id="settings-tab-content">
        <div id="tab-users" class="space-y-6"></div>
        <div id="tab-libraries" class="space-y-6 hidden"></div>
        <div id="tab-server" class="space-y-6 hidden"></div>
        <div id="tab-auth" class="space-y-6 hidden"></div>
        <div id="tab-backups" class="space-y-6 hidden"></div>
        <div id="tab-providers" class="space-y-6 hidden"></div>
        <div id="tab-upload" class="space-y-6 hidden"></div>
        <div id="tab-apikeys" class="space-y-6 hidden"></div>
        <div id="tab-listening-sessions" class="space-y-6 hidden"></div>
        <div id="tab-login-sessions" class="space-y-6 hidden"></div>
        <div id="tab-logs" class="space-y-6 hidden"></div>
        <div id="tab-notifications" class="space-y-6 hidden"></div>
        <div id="tab-feeds" class="space-y-6 hidden"></div>
        <div id="tab-emails" class="space-y-6 hidden"></div>
        <div id="tab-shares" class="space-y-6 hidden"></div>
      </div>
    </div>
  `;

  // Attach tab switcher click handlers
  const tabs = document.querySelectorAll('#settings-tabs button');
  tabs.forEach(tab => {
    tab.onclick = () => {
      tabs.forEach(t => {
        t.className = 'w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-black-50 hover:bg-black-500/30 hover:text-white flex items-center space-x-2';
      });
      tab.className = 'w-full text-left px-3 py-2 rounded-md font-semibold text-sm transition-colors text-accent bg-black-500/80 flex items-center space-x-2';

      const activeTabId = tab.dataset.tab;
      document.querySelectorAll('#settings-tab-content > div').forEach(content => {
        if (content.id === `tab-${activeTabId}`) {
          content.classList.remove('hidden');
        } else {
          content.classList.add('hidden');
        }
      });
    };
  });

  // Load respective tab details
  await Promise.all([
    renderUsersTab(),
    renderLibrariesTab(),
    renderServerSettingsTab(),
    renderAuthSettingsTab(),
    renderBackupsTab(),
    renderProvidersTab(),
    renderUploadTab(),
    renderApiKeysTab(),
    renderListeningSessionsTab(),
    renderLoginSessionsTab(),
    renderLogsTab(),
    renderNotificationsTab(),
    renderFeedsTab(),
    renderEmailsTab(),
    renderSharesTab()
  ]);
}

async function renderServerSettingsTab() {
  const container = document.getElementById('tab-server');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const settings = await request('GET', '/api/settings');
    const prefixes = settings.sortingPrefixes || ['a', 'the', 'an'];
    const corsValue = (settings.allowedCorsOrigins || '').split(',').map(s => s.trim()).filter(Boolean).join('\n');

    container.innerHTML = `
      <form id="server-settings-form" class="space-y-6 bg-primary border border-black-300 p-6 rounded-md">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Server Settings</h3>
        
        <!-- Category 1: General Settings -->
        <div class="space-y-4">
          <h4 class="text-md font-semibold text-accent">General Settings</h4>
          
          <div>
            <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Interface Language</label>
            <select id="setting-language" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
              <option value="en-us" ${settings.language === 'en-us' ? 'selected' : ''}>English (US)</option>
              <option value="es-es" ${settings.language === 'es-es' ? 'selected' : ''}>Español</option>
              <option value="fr-fr" ${settings.language === 'fr-fr' ? 'selected' : ''}>Français</option>
              <option value="de-de" ${settings.language === 'de-de' ? 'selected' : ''}>Deutsch</option>
            </select>
          </div>

          <div>
            <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Backups to Keep</label>
            <input type="number" id="setting-backups-to-keep" value="${settings.backupsToKeep || 2}" min="1" max="100" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            <p class="text-xs text-black-100 mt-1">Older backups are automatically pruned when new backups are created.</p>
          </div>

          <div>
            <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">OPDS Feed URL</label>
            <div class="flex space-x-2">
              <input type="text" id="setting-opds-url" readonly value="${window.location.origin}/opds" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none cursor-default">
              <button type="button" id="btn-copy-opds" class="bg-accent hover:opacity-90 text-primary font-bold px-3 py-2 rounded transition-opacity">Copy</button>
            </div>
            <p class="text-xs text-black-100 mt-1">Use this URL to connect your e-readers and book discovery clients (e.g. KyBook, Marvin, Aldiko) to Audiobookshelf.</p>
          </div>

          <div class="flex flex-col space-y-3 pt-2">
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-metadata-cover-with-item" ${settings.metadataCoverWithItem ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Embed cover image in item metadata folder</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-metadata-markdown-with-item" ${settings.metadataMarkdownWithItem ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Save metadata as markdown alongside media files</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-sorting-ignore-prefix" ${settings.sortingIgnorePrefix !== false ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Ignore title prefixes ("The", "A", "An", etc.) when sorting</span>
            </label>
          </div>
        </div>

        <hr class="border-black-400">

        <!-- Category 2: Scanner Settings -->
        <div class="space-y-4">
          <h4 class="text-md font-semibold text-accent">Scanner Settings</h4>

          <div class="flex flex-col space-y-3">
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-scanner-parse-subtitles" ${settings.scannerParseSubtitles !== false ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Parse subtitles from folders/filenames</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-scanner-find-covers" ${settings.scannerFindCovers !== false ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Automatically search for covers during scan</span>
            </label>
          </div>

          <div>
            <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Default Cover Provider</label>
            <select id="setting-scanner-cover-provider" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
              <option value="google" ${settings.scannerCoverProvider === 'google' ? 'selected' : ''}>Google Books</option>
              <option value="openlibrary" ${settings.scannerCoverProvider === 'openlibrary' ? 'selected' : ''}>Open Library</option>
              <option value="itunes" ${settings.scannerCoverProvider === 'itunes' ? 'selected' : ''}>iTunes</option>
              <option value="audible" ${settings.scannerCoverProvider === 'audible' ? 'selected' : ''}>Audible</option>
            </select>
          </div>

          <div class="flex flex-col space-y-3 pt-2">
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-scanner-prefer-matched-metadata" ${settings.scannerPreferMatchedMetadata ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Prefer matched metadata over embedded tags</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-watch-library-changes" ${settings.watchLibraryChanges !== false ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Watch library folders for changes</span>
            </label>
          </div>
        </div>

        <hr class="border-black-400">

        <!-- Category 3: Web Client Settings -->
        <div class="space-y-4">
          <h4 class="text-md font-semibold text-accent">Web Client Settings</h4>
          
          <div class="flex flex-col space-y-3">
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-chromecast-enabled" ${settings.chromecastEnabled ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Enable Chromecast support</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-allow-iframe" ${settings.allowIframe ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow embedding app in an iframe</span>
            </label>
          </div>
        </div>

        <hr class="border-black-400">

        <!-- Category 4: Display Settings -->
        <div class="space-y-4">
          <h4 class="text-md font-semibold text-accent">Display Settings</h4>

          <div class="flex flex-col space-y-3">
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-home-page-bookshelf-view" ${settings.homePageBookshelfView ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Show Home Page in Bookshelf View</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="setting-library-bookshelf-view" ${settings.libraryBookshelfView ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Show Library Page in Bookshelf View</span>
            </label>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Theme</label>
              <select id="setting-theme" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
                <option value="dark" ${settings.theme === 'dark' ? 'selected' : ''}>Dark (Default)</option>
                <option value="light" ${settings.theme === 'light' ? 'selected' : ''}>Light</option>
                <option value="sepia" ${settings.theme === 'sepia' ? 'selected' : ''}>Sepia</option>
              </select>
            </div>

            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Date Format</label>
              <select id="setting-date-format" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
                <option value="MM/DD/YYYY" ${settings.dateFormat === 'MM/DD/YYYY' ? 'selected' : ''}>MM/DD/YYYY</option>
                <option value="DD/MM/YYYY" ${settings.dateFormat === 'DD/MM/YYYY' ? 'selected' : ''}>DD/MM/YYYY</option>
                <option value="YYYY-MM-DD" ${settings.dateFormat === 'YYYY-MM-DD' ? 'selected' : ''}>YYYY-MM-DD</option>
              </select>
            </div>

            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Time Format</label>
              <select id="setting-time-format" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
                <option value="HH:mm" ${settings.timeFormat === 'HH:mm' ? 'selected' : ''}>24-Hour (HH:mm)</option>
                <option value="h:mm A" ${settings.timeFormat === 'h:mm A' ? 'selected' : ''}>12-Hour (h:mm AM/PM)</option>
              </select>
            </div>
          </div>
        </div>

        <hr class="border-black-400">

        <!-- Category 5: Security Settings -->
        <div class="space-y-4">
          <h4 class="text-md font-semibold text-accent">Security Settings</h4>

          <div>
            <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Allowed CORS Origins</label>
            <textarea id="setting-allowed-cors-origins" rows="3" placeholder="e.g. http://localhost:3000, https://myabs.com (comma- or newline-separated)" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent font-mono text-sm">${escapeHtml(corsValue)}</textarea>
            <p class="text-xs text-black-100 mt-1">Cross-Origin Resource Sharing domains allowed to access this server API.</p>
          </div>
        </div>

        <hr class="border-black-400">

        <!-- Category 6: Custom Styling & CSS -->
        <div class="space-y-4">
          <h4 class="text-md font-semibold text-accent">Custom Styling & CSS</h4>

          <div>
            <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Custom CSS</label>
            <textarea id="setting-custom-css" rows="6" placeholder="/* Custom CSS rule overrides e.g. :root { --color-accent: #ff00ff; } */" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent font-mono text-sm">${escapeHtml(settings.customCss || '')}</textarea>
            <p class="text-xs text-black-100 mt-1">Inject custom CSS styling rules into the web client.</p>
          </div>
        </div>

        <hr class="border-black-400">

        <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity">Save Server Settings</button>
      </form>

      <form id="sorting-prefixes-form" class="space-y-6 bg-primary border border-black-300 p-6 rounded-md">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Sorting Prefixes (Title Ignore Prefixes)</h3>
        <p class="text-sm text-black-100">Titles starting with these words followed by a space will ignore them when sorting. For example, "The Hobbit" will sort as "Hobbit".</p>
        
        <div>
          <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Prefixes (Comma Separated)</label>
          <input type="text" id="setting-prefixes" value="${escapeHtml(prefixes.join(', '))}" placeholder="e.g. the, a, an, el, la" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
        </div>

        <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity">Save & Recompute Prefixes</button>
      </form>

      <!-- Troubleshooting / Cache Tools -->
      <div class="space-y-6 bg-primary border border-black-300 p-6 rounded-md mt-6">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Troubleshooting / Cache Tools</h3>
        <p class="text-sm text-black-100">Perform maintenance operations on server caches and temporary storage.</p>
        
        <div class="flex flex-wrap gap-4 pt-2">
          <button type="button" id="btn-purge-all-cache" class="bg-black-600 hover:bg-black-500 text-white font-semibold px-4 py-2 rounded-md transition-colors border border-black-400/40">Purge All Cache</button>
          <button type="button" id="btn-purge-items-cache" class="bg-black-600 hover:bg-black-500 text-white font-semibold px-4 py-2 rounded-md transition-colors border border-black-400/40">Purge Items Cache</button>
        </div>
      </div>
    `;

    // Hook forms
    const btnCopyOpds = document.getElementById('btn-copy-opds');
    if (btnCopyOpds) {
      btnCopyOpds.onclick = () => {
        const opdsUrl = document.getElementById('setting-opds-url');
        if (opdsUrl) {
          opdsUrl.select();
          navigator.clipboard.writeText(opdsUrl.value).then(() => {
            alert('OPDS Feed URL copied to clipboard!');
          }).catch(err => {
            alert('Failed to copy: ' + err);
          });
        }
      };
    }

    document.getElementById('server-settings-form').onsubmit = async (e) => {
      e.preventDefault();
      try {
        const corsInput = document.getElementById('setting-allowed-cors-origins').value;
        const allowedCorsOrigins = corsInput.split(/[\n,]+/).map(s => s.trim()).filter(Boolean).join(',');

        const payload = {
          language: document.getElementById('setting-language').value,
          backupsToKeep: parseInt(document.getElementById('setting-backups-to-keep').value, 10),
          metadataCoverWithItem: document.getElementById('setting-metadata-cover-with-item').checked,
          metadataMarkdownWithItem: document.getElementById('setting-metadata-markdown-with-item').checked,
          sortingIgnorePrefix: document.getElementById('setting-sorting-ignore-prefix').checked,
          
          scannerParseSubtitles: document.getElementById('setting-scanner-parse-subtitles').checked,
          scannerFindCovers: document.getElementById('setting-scanner-find-covers').checked,
          scannerCoverProvider: document.getElementById('setting-scanner-cover-provider').value,
          scannerPreferMatchedMetadata: document.getElementById('setting-scanner-prefer-matched-metadata').checked,
          watchLibraryChanges: document.getElementById('setting-watch-library-changes').checked,
          
          chromecastEnabled: document.getElementById('setting-chromecast-enabled').checked,
          allowIframe: document.getElementById('setting-allow-iframe').checked,
          
          homePageBookshelfView: document.getElementById('setting-home-page-bookshelf-view').checked,
          libraryBookshelfView: document.getElementById('setting-library-bookshelf-view').checked,
          dateFormat: document.getElementById('setting-date-format').value,
          timeFormat: document.getElementById('setting-time-format').value,
          theme: document.getElementById('setting-theme').value,
          customCss: document.getElementById('setting-custom-css').value,
          
          allowedCorsOrigins: allowedCorsOrigins
        };

        const res = await request('PATCH', '/api/settings', payload);
        if (res && res.serverSettings) {
          window.serverSettings = res.serverSettings;
          applyServerThemeAndCss(res.serverSettings);
        }
        alert('Server settings saved successfully!');
      } catch (err) {
        alert('Failed to save settings: ' + err.message);
      }
    };

    document.getElementById('sorting-prefixes-form').onsubmit = async (e) => {
      e.preventDefault();
      try {
        const val = document.getElementById('setting-prefixes').value;
        const prefixArray = val.split(',').map(s => s.trim()).filter(Boolean);
        const res = await request('PATCH', '/api/sorting-prefixes', { sortingPrefixes: prefixArray });
        if (res && res.serverSettings) {
          window.serverSettings = res.serverSettings;
        }
        alert(`Sorting prefixes updated! Title ignore columns will update in the background.`);
      } catch (err) {
        alert('Failed to save prefixes: ' + err.message);
      }
    };

    const btnPurgeAll = document.getElementById('btn-purge-all-cache');
    if (btnPurgeAll) {
      btnPurgeAll.onclick = async () => {
        if (!confirm('Are you sure you want to purge all cache? This includes resized cover images.')) return;
        try {
          btnPurgeAll.disabled = true;
          btnPurgeAll.textContent = 'Purging...';
          await request('POST', '/api/cache/purge-all');
          alert('Cache purged successfully!');
        } catch (err) {
          alert('Failed to purge cache: ' + err.message);
        } finally {
          btnPurgeAll.disabled = false;
          btnPurgeAll.textContent = 'Purge All Cache';
        }
      };
    }

    const btnPurgeItems = document.getElementById('btn-purge-items-cache');
    if (btnPurgeItems) {
      btnPurgeItems.onclick = async () => {
        if (!confirm('Are you sure you want to purge item cover cache? All resized cover images will be deleted.')) return;
        try {
          btnPurgeItems.disabled = true;
          btnPurgeItems.textContent = 'Purging...';
          await request('POST', '/api/cache/purge-items');
          alert('Items cover cache purged successfully!');
        } catch (err) {
          alert('Failed to purge items cover cache: ' + err.message);
        } finally {
          btnPurgeItems.disabled = false;
          btnPurgeItems.textContent = 'Purge Items Cache';
        }
      };
    }

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm">Failed to load server settings: ${err.message}</p>`;
  }
}

async function renderAuthSettingsTab() {
  const container = document.getElementById('tab-auth');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const auth = await request('GET', '/api/auth-settings');
    const activeMethods = auth.authActiveAuthMethods || ['local'];

    container.innerHTML = `
      <form id="auth-settings-form" class="space-y-6 bg-primary border border-black-300 p-6 rounded-md">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Authentication & OpenID Connect</h3>
        
        <div>
          <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Active Authentication Methods</label>
          <div class="flex items-center space-x-6 text-sm">
            <label class="flex items-center space-x-3 cursor-pointer">
              <span class="abs-switch">
                <input type="checkbox" id="auth-method-local" value="local" ${activeMethods.includes('local') ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Local Accounts</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer">
              <span class="abs-switch">
                <input type="checkbox" id="auth-method-openid" value="openid" ${activeMethods.includes('openid') ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>OpenID Connect (SSO)</span>
            </label>
          </div>
        </div>

        <div id="oidc-fields-container" class="${activeMethods.includes('openid') ? '' : 'opacity-40 pointer-events-none'} transition-all space-y-4 pt-4 border-t border-black-400">
          <h4 class="text-md font-semibold text-black-100">OIDC Client Configuration</h4>
          
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="block text-xs text-black-100 mb-1">Issuer URL</label>
              <input type="text" id="oidc-issuer" value="${escapeHtml(auth.authOpenIDIssuerURL || '')}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Button Text</label>
              <input type="text" id="oidc-button-text" value="${escapeHtml(auth.authOpenIDButtonText || '')}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Client ID</label>
              <input type="text" id="oidc-client-id" value="${escapeHtml(auth.authOpenIDClientID || '')}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Client Secret</label>
              <input type="password" id="oidc-client-secret" value="${escapeHtml(auth.authOpenIDClientSecret || '')}" placeholder="••••••••" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Custom Login Message</label>
              <input type="text" id="oidc-custom-message" value="${escapeHtml(auth.authLoginCustomMessage || '')}" placeholder="Optional custom message on login screen" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Mobile Redirect URI</label>
              <input type="text" id="oidc-mobile-redirect" value="${escapeHtml((auth.authOpenIDMobileRedirectURIs || []).join(', '))}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
          </div>
          
          <div class="flex flex-col md:flex-row md:items-center md:space-x-6 space-y-3 md:space-y-0 pt-2 text-sm">
            <label class="flex items-center space-x-3 cursor-pointer">
              <span class="abs-switch">
                <input type="checkbox" id="oidc-autolaunch" ${auth.authOpenIDAutoLaunch ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Auto-Launch OpenID (Skips login form)</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer">
              <span class="abs-switch">
                <input type="checkbox" id="oidc-autoregister" ${auth.authOpenIDAutoRegister ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Auto-Register New Users</span>
            </label>
          </div>
        </div>

        <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity">Save Auth Settings</button>
      </form>
    `;

    // Dynamic OIDC container locking toggle
    const openidCheckbox = document.getElementById('auth-method-openid');
    openidCheckbox.onchange = () => {
      const container = document.getElementById('oidc-fields-container');
      if (openidCheckbox.checked) {
        container.classList.remove('opacity-40', 'pointer-events-none');
      } else {
        container.classList.add('opacity-40', 'pointer-events-none');
      }
    };

    document.getElementById('auth-settings-form').onsubmit = async (e) => {
      e.preventDefault();
      try {
        const methods = [];
        if (document.getElementById('auth-method-local').checked) methods.push('local');
        if (document.getElementById('auth-method-openid').checked) methods.push('openid');

        if (methods.length === 0) {
          alert('You must enable at least one authentication method.');
          return;
        }

        const payload = {
          authActiveAuthMethods: methods,
          authOpenIDIssuerURL: document.getElementById('oidc-issuer').value,
          authOpenIDButtonText: document.getElementById('oidc-button-text').value,
          authOpenIDClientID: document.getElementById('oidc-client-id').value,
          authLoginCustomMessage: document.getElementById('oidc-custom-message').value,
          authOpenIDAutoLaunch: document.getElementById('oidc-autolaunch').checked,
          authOpenIDAutoRegister: document.getElementById('oidc-autoregister').checked
        };

        const secretVal = document.getElementById('oidc-client-secret').value;
        if (secretVal && secretVal !== '••••••••') {
          payload.authOpenIDClientSecret = secretVal;
        }

        const mobileRedirects = document.getElementById('oidc-mobile-redirect').value;
        payload.authOpenIDMobileRedirectURIs = mobileRedirects.split(',').map(s => s.trim()).filter(Boolean);

        await request('PATCH', '/api/auth-settings', payload);
        alert('Authentication settings saved successfully!');
      } catch (err) {
        alert('Failed to save auth settings: ' + err.message);
      }
    };

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm">Failed to load auth settings: ${err.message}</p>`;
  }
}

async function renderBackupsTab() {
  const container = document.getElementById('tab-backups');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const [backupPayload, serverSettings] = await Promise.all([
      request('GET', '/api/backups'),
      request('GET', '/api/settings')
    ]);
    const backups = backupPayload.backups || [];
    const location = backupPayload.backupLocation || '';
    const backupSchedule = serverSettings.backupSchedule || '';

    // Determine initial dropdown selection
    let initialPreset = '';
    if (backupSchedule === '') {
      initialPreset = '';
    } else if (backupSchedule === '0 * * * *') {
      initialPreset = 'hourly';
    } else if (backupSchedule === '0 0 * * *') {
      initialPreset = 'daily';
    } else if (backupSchedule === '0 0 * * 0') {
      initialPreset = 'weekly';
    } else {
      initialPreset = 'custom';
    }

    container.innerHTML = `
      <div class="bg-primary border border-black-300 p-6 rounded-md space-y-6">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Backup Settings</h3>
        
        <form id="backup-path-form" class="flex items-end space-x-4">
          <div class="flex-grow">
            <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Backups Storage Directory</label>
            <input type="text" id="backup-location-path" value="${escapeHtml(location)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
          </div>
          <button type="submit" class="bg-black-400 hover:bg-black-300 border border-black-300 text-white font-medium px-4 py-2 rounded transition-colors">Change Path</button>
        </form>

        <form id="backup-schedule-form" class="pt-4 border-t border-black-400 space-y-4">
          <div class="flex flex-col md:flex-row md:items-end md:space-x-4 space-y-4 md:space-y-0">
            <div class="flex-1">
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Backup Schedule Preset</label>
              <select id="backup-schedule-preset" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
                <option value="" ${initialPreset === '' ? 'selected' : ''}>Disabled</option>
                <option value="hourly" ${initialPreset === 'hourly' ? 'selected' : ''}>Hourly</option>
                <option value="daily" ${initialPreset === 'daily' ? 'selected' : ''}>Daily (Midnight)</option>
                <option value="weekly" ${initialPreset === 'weekly' ? 'selected' : ''}>Weekly (Sunday Midnight)</option>
                <option value="custom" ${initialPreset === 'custom' ? 'selected' : ''}>Custom Cron Expression</option>
              </select>
            </div>
            <div id="custom-cron-container" class="flex-1 ${initialPreset === 'custom' ? '' : 'hidden'}">
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Custom Cron Expression</label>
              <input type="text" id="backup-schedule-cron" value="${escapeHtml(backupSchedule)}" placeholder="e.g. 0 0 * * *" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <button type="submit" class="bg-black-400 hover:bg-black-300 border border-black-300 text-white font-medium px-4 py-2 rounded transition-colors w-full md:w-auto">Save Schedule</button>
            </div>
          </div>
          <p class="text-xs text-black-100">Configure background scheduled backups to run automatically using standard cron formats.</p>
        </form>

        <div class="flex justify-between items-center pt-4 border-t border-black-400">
          <div>
            <h4 class="text-sm font-semibold">Manual System Backup</h4>
            <p class="text-xs text-black-100">Saves the SQLite database and all book cover metadata into a zip package.</p>
          </div>
          <button id="create-backup-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity flex items-center space-x-1.5">
            <span class="material-symbols text-lg">backup</span>
            <span>Create Backup Now</span>
          </button>
        </div>

        <div class="pt-4 border-t border-black-400">
          <h4 class="text-sm font-semibold mb-2">Upload Backup File</h4>
          <input type="file" id="upload-backup-file" accept=".audiobookshelf" class="text-sm text-black-50 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-xs file:font-semibold file:bg-black-400 file:text-white hover:file:bg-black-300 cursor-pointer">
        </div>
      </div>

      <div class="bg-primary border border-black-300 p-6 rounded-md">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-4">Available Backups</h3>
        
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm text-black-50">
            <thead>
              <tr class="border-b border-black-400/60 text-xs text-black-100 uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">Date</th>
                <th class="px-4 py-3">Filename</th>
                <th class="px-4 py-3">Size</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="backups-list-rows" class="divide-y divide-black-400/50">
              <!-- Render rows -->
            </tbody>
          </table>
          <div id="backups-empty-state" class="text-center py-6 text-black-100 hidden">No backups found in directory.</div>
        </div>
      </div>
    `;

    renderBackupsListRows(backups);

    // Setup Event Handlers
    const presetSelect = document.getElementById('backup-schedule-preset');
    const customCronContainer = document.getElementById('custom-cron-container');
    const customCronInput = document.getElementById('backup-schedule-cron');

    presetSelect.onchange = () => {
      if (presetSelect.value === 'custom') {
        customCronContainer.classList.remove('hidden');
      } else {
        customCronContainer.classList.add('hidden');
      }
    };

    document.getElementById('backup-schedule-form').onsubmit = async (e) => {
      e.preventDefault();
      try {
        let scheduleVal = '';
        if (presetSelect.value === 'custom') {
          scheduleVal = customCronInput.value.trim();
        } else if (presetSelect.value === 'hourly') {
          scheduleVal = '0 * * * *';
        } else if (presetSelect.value === 'daily') {
          scheduleVal = '0 0 * * *';
        } else if (presetSelect.value === 'weekly') {
          scheduleVal = '0 0 * * 0';
        }

        await request('PATCH', '/api/settings', { backupSchedule: scheduleVal });
        alert('Backup schedule updated successfully!');
        renderBackupsTab(); // reload
      } catch (err) {
        alert('Failed to update backup schedule: ' + err.message);
      }
    };

    document.getElementById('backup-path-form').onsubmit = async (e) => {
      e.preventDefault();
      try {
        const path = document.getElementById('backup-location-path').value;
        await request('PATCH', '/api/backups/path', { path });
        alert('Backup path updated successfully!');
        renderBackupsTab(); // reload
      } catch (err) {
        alert('Failed to update backup path: ' + err.message);
      }
    };

    document.getElementById('create-backup-btn').onclick = async () => {
      const btn = document.getElementById('create-backup-btn');
      btn.disabled = true;
      btn.innerHTML = `<span class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary mr-1.5"></span><span>Creating...</span>`;
      try {
        const res = await request('POST', '/api/backups');
        renderBackupsListRows(res.backups || []);
        alert('Backup created successfully!');
      } catch (err) {
        alert('Failed to create backup: ' + err.message);
      } finally {
        btn.disabled = false;
        btn.innerHTML = `<span class="material-symbols text-lg">backup</span><span>Create Backup Now</span>`;
      }
    };

    document.getElementById('upload-backup-file').onchange = async (e) => {
      const file = e.target.files[0];
      if (!file) return;

      const formData = new FormData();
      formData.append('file', file);

      try {
        const token = localStorage.getItem('token');
        const uploadUrl = resolvePath('/api/backups/upload');
        const res = await fetch(uploadUrl, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${token}`
          },
          body: formData
        });
        if (!res.ok) throw new Error(await res.text() || res.statusText);
        const data = await res.json();
        renderBackupsListRows(data.backups || []);
        alert('Backup uploaded successfully!');
      } catch (err) {
        alert('Upload failed: ' + err.message);
      }
    };

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm">Failed to load backups: ${err.message}</p>`;
  }
}

function renderBackupsListRows(backups) {
  const tbody = document.getElementById('backups-list-rows');
  const emptyState = document.getElementById('backups-empty-state');
  if (!tbody) return;

  tbody.innerHTML = '';
  if (backups.length === 0) {
    emptyState.classList.remove('hidden');
    return;
  }
  emptyState.classList.add('hidden');

  backups.forEach(b => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-black-500/30';

    const sizeFormatted = (b.fileSize / (1024 * 1024)).toFixed(2) + ' MB';
    
    // Download link
    const downloadUrl = resolvePath(`/api/backups/${b.id}/download?token=${localStorage.getItem('token')}`);

    tr.innerHTML = `
      <td class="px-4 py-3 font-medium text-white">${b.datePretty}</td>
      <td class="px-4 py-3 font-mono text-xs">${escapeHtml(b.filename)}</td>
      <td class="px-4 py-3">${sizeFormatted}</td>
      <td class="px-4 py-3 text-right space-x-2">
        <button class="apply-btn bg-emerald-800 hover:bg-emerald-700 text-emerald-100 text-xs font-semibold px-2.5 py-1 rounded" data-id="${b.id}">Restore</button>
        <a href="${downloadUrl}" class="inline-block bg-black-400 hover:bg-black-300 text-white text-xs font-semibold px-2.5 py-1 rounded">Download</a>
        <button class="delete-btn bg-red-900 hover:bg-red-800 text-red-200 text-xs font-semibold px-2.5 py-1 rounded" data-id="${b.id}">Delete</button>
      </td>
    `;

    // Bind triggers
    tr.querySelector('.apply-btn').onclick = async () => {
      if (!confirm(`Are you absolutely sure you want to restore the backup from ${b.datePretty}? This will disconnect current sessions, overwrite the database, and trigger a server reload.`)) {
        return;
      }
      try {
        await request('POST', `/api/backups/${b.id}/apply`);
        alert('Backup applied successfully! Page will reload.');
        window.location.reload();
      } catch (err) {
        alert('Restore failed: ' + err.message);
      }
    };

    tr.querySelector('.delete-btn').onclick = async () => {
      if (!confirm(`Delete backup file ${b.filename}?`)) return;
      try {
        const res = await request('DELETE', `/api/backups/${b.id}`);
        renderBackupsListRows(res.backups || []);
        alert('Backup deleted.');
      } catch (err) {
        alert('Delete failed: ' + err.message);
      }
    };

    tbody.appendChild(tr);
  });
}

async function renderProvidersTab() {
  const container = document.getElementById('tab-providers');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const providersPayload = await request('GET', '/api/search/providers');
    const customPayload = await request('GET', '/api/custom-metadata-providers');
    
    const booksProviders = providersPayload.providers?.books || [];
    const podcastProviders = providersPayload.providers?.podcasts || [];
    const customProviders = customPayload.providers || [];

    container.innerHTML = `
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <!-- Providers Lists -->
        <div class="bg-primary border border-black-300 p-6 rounded-md space-y-4">
          <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Active Metadata Providers</h3>
          
          <div>
            <h4 class="text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Book Metadata</h4>
            <ul class="space-y-1 text-sm list-disc pl-5">
              ${booksProviders.map(p => `<li>${escapeHtml(p.text)} <span class="text-xs text-black-100">(${escapeHtml(p.value)})</span></li>`).join('')}
            </ul>
          </div>
          
          <div class="pt-4 border-t border-black-400">
            <h4 class="text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Podcast Metadata</h4>
            <ul class="space-y-1 text-sm list-disc pl-5">
              ${podcastProviders.map(p => `<li>${escapeHtml(p.text)} <span class="text-xs text-black-100">(${escapeHtml(p.value)})</span></li>`).join('')}
            </ul>
          </div>
        </div>

        <!-- Add Custom Provider Form -->
        <form id="create-provider-form" class="bg-primary border border-black-300 p-6 rounded-md space-y-4">
          <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Add Custom Provider</h3>
          
          <div>
            <label class="block text-xs text-black-100 mb-1">Provider Name</label>
            <input type="text" id="prov-name" required placeholder="My Custom Search" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
          </div>
          <div>
            <label class="block text-xs text-black-100 mb-1">Endpoint URL</label>
            <input type="url" id="prov-url" required placeholder="https://api.myprovider.com/search" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
          </div>
          <div>
            <label class="block text-xs text-black-100 mb-1">Media Type</label>
            <select id="prov-mediatype" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
              <option value="book">Book</option>
              <option value="podcast">Podcast</option>
            </select>
          </div>
          <div>
            <label class="block text-xs text-black-100 mb-1">Authorization Header Value (Optional)</label>
            <input type="text" id="prov-auth" placeholder="Bearer my-secret-token" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
          </div>

          <button type="submit" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-2 rounded transition-opacity text-sm">Add Provider</button>
        </form>
      </div>

      <!-- Custom Providers List -->
      <div class="bg-primary border border-black-300 p-6 rounded-md">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-4">Custom Providers</h3>
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm text-black-50">
            <thead>
              <tr class="border-b border-black-400/60 text-xs text-black-100 uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">Name</th>
                <th class="px-4 py-3">Media Type</th>
                <th class="px-4 py-3">URL</th>
                <th class="px-4 py-3 text-right">Action</th>
              </tr>
            </thead>
            <tbody id="custom-providers-rows" class="divide-y divide-black-400/50">
              <!-- Render custom rows -->
            </tbody>
          </table>
          <div id="custom-providers-empty" class="text-center py-6 text-black-100 hidden">No custom providers configured.</div>
        </div>
      </div>
    `;

    renderCustomProvidersRows(customProviders);

    document.getElementById('create-provider-form').onsubmit = async (e) => {
      e.preventDefault();
      try {
        const payload = {
          name: document.getElementById('prov-name').value,
          url: document.getElementById('prov-url').value,
          mediaType: document.getElementById('prov-mediatype').value,
        };
        const authHeader = document.getElementById('prov-auth').value;
        if (authHeader) {
          payload.authHeaderValue = authHeader;
        }

        await request('POST', '/api/custom-metadata-providers', payload);
        alert('Custom metadata provider created!');
        renderProvidersTab(); // reload
      } catch (err) {
        alert('Failed to add provider: ' + err.message);
      }
    };

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm">Failed to load metadata providers: ${err.message}</p>`;
  }
}

function renderCustomProvidersRows(customProviders) {
  const tbody = document.getElementById('custom-providers-rows');
  const emptyState = document.getElementById('custom-providers-empty');
  if (!tbody) return;

  tbody.innerHTML = '';
  if (customProviders.length === 0) {
    emptyState.classList.remove('hidden');
    return;
  }
  emptyState.classList.add('hidden');

  customProviders.forEach(p => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-black-500/30';

    tr.innerHTML = `
      <td class="px-4 py-3 font-semibold text-white">${escapeHtml(p.name)}</td>
      <td class="px-4 py-3 uppercase text-xs">${escapeHtml(p.mediaType)}</td>
      <td class="px-4 py-3 font-mono text-xs truncate max-w-xs">${escapeHtml(p.url)}</td>
      <td class="px-4 py-3 text-right">
        <button class="delete-prov-btn bg-red-900 hover:bg-red-800 text-red-200 text-xs font-semibold px-2.5 py-1 rounded" data-id="${p.id}">Delete</button>
      </td>
    `;

    tr.querySelector('.delete-prov-btn').onclick = async () => {
      if (!confirm(`Are you sure you want to delete custom provider "${p.name}"? Any libraries using it will fallback to defaults.`)) return;
      try {
        await request('DELETE', `/api/custom-metadata-providers/${p.id}`);
        renderProvidersTab(); // reload
        alert('Custom provider deleted.');
      } catch (err) {
        alert('Delete failed: ' + err.message);
      }
    };

    tbody.appendChild(tr);
  });
}

function escapeHtml(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

/**
 * Render the Upload Media tab.
 * Provides a file upload form targeting POST /api/libraries/{id}/items.
 * This follows the standard audiobookshelf upload API contract.
 */
async function renderUploadTab() {
  const container = document.getElementById('tab-upload');
  if (!container) return;

  container.innerHTML = `
    <div class="bg-primary border border-black-300 p-6 rounded-md space-y-6">
      <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Upload Media to Library</h3>
      <p class="text-sm text-black-100">
        Upload audio files or e-book files directly to the active library.
        Supported formats include MP3, M4B, OGG, AAC, EPUB, and PDF.
        Files will be processed and added to the library after upload.
      </p>

      <form id="upload-media-form" class="space-y-4">
        <div>
          <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Select Files</label>
          <input type="file" id="upload-media-files" multiple
            accept="audio/*,.m4b,.epub,.pdf,.azw3,.mobi"
            class="text-sm text-black-50 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-xs file:font-semibold file:bg-black-400 file:text-white hover:file:bg-black-300 cursor-pointer w-full">
          <p class="text-xs text-black-100 mt-1">Hold Ctrl/Cmd to select multiple files.</p>
        </div>

        <div id="upload-file-list" class="hidden space-y-1">
          <p class="text-xs font-semibold text-black-100 uppercase tracking-wider">Selected Files:</p>
          <ul id="upload-file-names" class="text-sm text-black-50 list-disc pl-5 space-y-0.5"></ul>
        </div>

        <div id="upload-progress-bar-container" class="hidden">
          <div class="w-full bg-black-400 rounded-full h-2">
            <div id="upload-progress-bar" class="bg-accent h-2 rounded-full transition-all duration-300" style="width: 0%"></div>
          </div>
          <p id="upload-progress-label" class="text-xs text-black-100 mt-1">Uploading...</p>
        </div>

        <button type="submit" id="upload-media-btn"
          class="flex items-center space-x-2 bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity">
          <span class="material-symbols text-lg">upload</span>
          <span>Upload to Active Library</span>
        </button>
      </form>
    </div>
  `;

  // Wire up file selection preview
  const fileInput = document.getElementById('upload-media-files');
  const fileList = document.getElementById('upload-file-list');
  const fileNames = document.getElementById('upload-file-names');

  fileInput.onchange = () => {
    fileNames.innerHTML = '';
    if (fileInput.files.length === 0) {
      fileList.classList.add('hidden');
      return;
    }
    fileList.classList.remove('hidden');
    Array.from(fileInput.files).forEach(f => {
      const li = document.createElement('li');
      li.textContent = f.name;
      fileNames.appendChild(li);
    });
  };

  // Wire up submit
  const form = document.getElementById('upload-media-form');
  form.onsubmit = async (e) => {
    e.preventDefault();
    const files = fileInput.files;
    if (!files || files.length === 0) {
      alert('Please select at least one file to upload.');
      return;
    }

    const libraryId = getActiveLibraryId();
    if (!libraryId) {
      alert('No active library selected. Please select a library first.');
      return;
    }

    const btn = document.getElementById('upload-media-btn');
    const progressContainer = document.getElementById('upload-progress-bar-container');
    const progressBar = document.getElementById('upload-progress-bar');
    const progressLabel = document.getElementById('upload-progress-label');

    btn.disabled = true;
    progressContainer.classList.remove('hidden');
    progressBar.style.width = '0%';
    progressLabel.textContent = 'Uploading...';

    const formData = new FormData();
    Array.from(files).forEach(f => formData.append('files', f));

    try {
      const token = localStorage.getItem('token');
      const uploadUrl = resolvePath(`/api/libraries/${libraryId}/items`);

      // Use XMLHttpRequest for progress tracking
      await new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.open('POST', uploadUrl);
        xhr.setRequestHeader('Authorization', `Bearer ${token}`);

        xhr.upload.onprogress = (evt) => {
          if (evt.lengthComputable) {
            const pct = Math.round((evt.loaded / evt.total) * 100);
            progressBar.style.width = `${pct}%`;
            progressLabel.textContent = `Uploading... ${pct}%`;
          }
        };

        xhr.onload = () => {
          if (xhr.status >= 200 && xhr.status < 300) {
            resolve(xhr.response);
          } else {
            reject(new Error(xhr.responseText || `HTTP ${xhr.status}`));
          }
        };
        xhr.onerror = () => reject(new Error('Network error during upload'));
        xhr.send(formData);
      });

      progressBar.style.width = '100%';
      progressLabel.textContent = 'Upload complete!';
      fileInput.value = '';
      fileNames.innerHTML = '';
      fileList.classList.add('hidden');
      alert(`${files.length} file(s) uploaded successfully! The library will be scanned for new items.`);
    } catch (err) {
      progressLabel.textContent = 'Upload failed: ' + err.message;
      alert('Upload failed: ' + err.message);
    } finally {
      btn.disabled = false;
    }
  };
}

async function renderUsersTab() {
  const container = document.getElementById('tab-users');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const [usersRes, currentUser] = await Promise.all([
      request('GET', '/api/users'),
      request('GET', '/api/me')
    ]);
    const users = usersRes.users || [];

    container.innerHTML = `
      <div class="bg-primary border border-black-300 p-6 rounded-md space-y-6">
        <div class="flex justify-between items-center border-b border-black-400 pb-4">
          <div>
            <h3 class="text-lg font-semibold">Users</h3>
            <p class="text-xs text-black-100 mt-1">Manage user accounts, authentication types, and access permissions.</p>
          </div>
          <button id="add-user-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity flex items-center space-x-1.5 text-sm">
            <span class="material-symbols text-lg">person_add</span>
            <span>Add User</span>
          </button>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm text-black-50">
            <thead>
              <tr class="border-b border-black-400/60 text-xs text-black-100 uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">Username</th>
                <th class="px-4 py-3">Account Type</th>
                <th class="px-4 py-3">Last Seen</th>
                <th class="px-4 py-3">Created At</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="users-list-rows" class="divide-y divide-black-400/50"></tbody>
          </table>
          <div id="users-empty-state" class="text-center py-6 text-black-100 hidden">No users found.</div>
        </div>
      </div>
    `;

    renderUsersListRows(users, currentUser);

    document.getElementById('add-user-btn').onclick = () => {
      triggerUserModal(null, currentUser, () => renderUsersTab());
    };

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm">Failed to load users: ${err.message}</p>`;
  }
}

function renderUsersListRows(users, currentUser) {
  const tbody = document.getElementById('users-list-rows');
  const emptyState = document.getElementById('users-empty-state');
  if (!tbody) return;

  tbody.innerHTML = '';
  if (users.length === 0) {
    emptyState.classList.remove('hidden');
    return;
  }
  emptyState.classList.add('hidden');

  users.forEach(u => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-black-500/30';

    const lastSeenFormatted = u.lastSeen ? window.formatDateTime(u.lastSeen) : 'Never';
    const createdAtFormatted = u.createdAt ? window.formatDateTime(u.createdAt) : 'Unknown';
    
    let typeDisplay = u.type || 'user';
    if (typeDisplay === 'root') typeDisplay = 'Root Admin';
    else if (typeDisplay === 'admin') typeDisplay = 'Admin';
    else typeDisplay = 'User';

    const canDelete = u.type !== 'root' && u.id !== currentUser.id;
    const canEdit = currentUser.type === 'root' || (currentUser.type === 'admin' && u.type !== 'root');

    tr.innerHTML = `
      <td class="px-4 py-3 font-semibold text-white flex items-center space-x-2">
        <span>${escapeHtml(u.username)}</span>
        ${u.isActive ? '' : '<span class="bg-red-900/50 text-red-200 text-[10px] px-1.5 py-0.5 rounded font-normal uppercase">Inactive</span>'}
        ${u.hasOpenIDLink ? '<span class="bg-blue-900/50 text-blue-200 text-[10px] px-1.5 py-0.5 rounded font-normal uppercase">OIDC</span>' : ''}
      </td>
      <td class="px-4 py-3 text-xs capitalize">${typeDisplay}</td>
      <td class="px-4 py-3 text-xs text-black-100">${lastSeenFormatted}</td>
      <td class="px-4 py-3 text-xs text-black-100">${createdAtFormatted}</td>
      <td class="px-4 py-3 text-right space-x-2">
        ${u.hasOpenIDLink && canEdit ? '<button class="unlink-oidc-btn bg-yellow-900 hover:bg-yellow-800 text-yellow-200 text-xs font-semibold px-2 py-1 rounded" data-id="' + u.id + '">Unlink</button>' : ''}
        <button class="edit-user-btn bg-black-400 hover:bg-black-300 text-white text-xs font-semibold px-2.5 py-1 rounded disabled:opacity-40 disabled:cursor-not-allowed" ${canEdit ? '' : 'disabled'} data-id="${u.id}">Edit</button>
        <button class="delete-user-btn bg-red-900 hover:bg-red-800 text-red-200 text-xs font-semibold px-2.5 py-1 rounded disabled:opacity-40 disabled:cursor-not-allowed" ${canDelete ? '' : 'disabled'} data-id="${u.id}">Delete</button>
      </td>
    `;

    if (canEdit) {
      tr.querySelector('.edit-user-btn').onclick = () => {
        triggerUserModal(u, currentUser, () => renderUsersTab());
      };
      if (u.hasOpenIDLink) {
        tr.querySelector('.unlink-oidc-btn').onclick = async () => {
          if (!confirm(`Are you sure you want to unlink OIDC for user "${u.username}"?`)) return;
          try {
            await request('PATCH', `/api/users/${u.id}/openid-unlink`);
            renderUsersTab();
          } catch (err) {
            alert('Failed to unlink OpenID: ' + err.message);
          }
        };
      }
    }

    if (canDelete) {
      tr.querySelector('.delete-user-btn').onclick = async () => {
        if (!confirm(`Are you sure you want to delete user "${u.username}"? This will delete all of their history and listening progress.`)) {
          return;
        }
        try {
          await request('DELETE', `/api/users/${u.id}`);
          renderUsersTab();
        } catch (err) {
          alert('Failed to delete user: ' + err.message);
        }
      };
    }

    tbody.appendChild(tr);
  });
}

async function triggerUserModal(user = null, currentUser, onSaveSuccess) {
  const isEdit = !!user;
  const libraries = getLibrariesList();

  let allTags = [];
  try {
    const res = await request('GET', '/api/tags');
    allTags = res.tags || [];
  } catch (err) {
    console.error('Failed to load tags for user permissions', err);
  }

  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 overflow-y-auto';

  const perms = user?.permissions || {
    download: true,
    accessExplicitContent: false,
    accessAllLibraries: true,
    accessAllTags: true,
    selectedTagsNotAccessible: false
  };
  const libsAccessible = user?.librariesAccessible || [];
  const itemTagsSelected = user?.itemTagsSelected || [];

  const accessAllTags = perms.accessAllTags !== false;
  const selectedTagsNotAccessible = perms.selectedTagsNotAccessible === true;

  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-lg p-6 rounded-md shadow-lg space-y-4 my-8">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2">${isEdit ? 'Edit User' : 'Add User'}</h3>
      
      <form id="user-form" class="space-y-4">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-xs text-black-100 mb-1">Username</label>
            <input type="text" id="user-username" required value="${isEdit ? escapeHtml(user.username) : ''}" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm" ${isEdit ? 'disabled' : ''}>
          </div>
          <div>
            <label class="block text-xs text-black-100 mb-1">Email Address</label>
            <input type="email" id="user-email" value="${isEdit && user.email ? escapeHtml(user.email) : ''}" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
          </div>
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">Password</label>
          <input type="password" id="user-password" ${isEdit ? '' : 'required'} placeholder="${isEdit ? '•••••••• (leave blank to keep current)' : 'Enter password'}" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-xs text-black-100 mb-1">Account Type</label>
            <select id="user-type" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm" ${isEdit && user.type === 'root' ? 'disabled' : ''}>
              <option value="user" ${isEdit && user.type === 'user' ? 'selected' : ''}>Regular User</option>
              <option value="admin" ${isEdit && user.type === 'admin' ? 'selected' : ''}>Admin</option>
              ${isEdit && user.type === 'root' ? '<option value="root" selected>Root Admin</option>' : ''}
            </select>
          </div>
          <div class="flex items-end pb-2">
            <label class="flex items-center space-x-3 cursor-pointer text-sm ${isEdit && user.type === 'root' ? 'opacity-50 pointer-events-none' : ''}">
              <span class="abs-switch">
                <input type="checkbox" id="user-isactive" ${!isEdit || user.isActive ? 'checked' : ''} ${isEdit && user.type === 'root' ? 'disabled' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Account is Active</span>
            </label>
          </div>
        </div>

        <!-- Permissions Section -->
        <div class="border-t border-black-400 pt-3 space-y-3">
          <h4 class="text-xs font-semibold text-accent uppercase tracking-wider">Permissions</h4>
          
          <div class="flex flex-col space-y-3">
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-download" ${perms.download !== false ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow downloading files</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-explicit" ${perms.accessExplicitContent ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow explicit content access</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-upload" ${perms.upload === true ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow uploading files</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-delete" ${perms.delete === true ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow deleting media</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-update" ${perms.update === true ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow editing metadata / library scans</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-rss" ${perms.accessRss === true ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow accessing RSS feeds</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-shares" ${perms.createShares === true ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow creating public shares</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-all-libraries" ${perms.accessAllLibraries !== false ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow access to all libraries</span>
            </label>
          </div>

          <!-- Library Selector (Collapsible) -->
          <div id="library-selector-container" class="${perms.accessAllLibraries !== false ? 'hidden' : ''} border border-black-300 rounded p-2 bg-black-500 max-h-36 overflow-y-auto space-y-1.5">
            <p class="text-[10px] font-semibold text-black-100 uppercase mb-1">Select Libraries:</p>
            ${libraries.length === 0 ? '<p class="text-xs text-black-100">No libraries available</p>' : libraries.map(lib => `
              <label class="flex items-center space-x-2 text-xs cursor-pointer hover:bg-black-400 p-1 rounded">
                <input type="checkbox" value="${lib.id}" ${libsAccessible.includes(lib.id) ? 'checked' : ''} class="user-lib-checkbox rounded text-accent bg-black-600 border-black-300">
                <span>${escapeHtml(lib.name)}</span>
              </label>
            `).join('')}
          </div>
        </div>

        <!-- Tag Restrictions Section -->
        <div class="border-t border-black-400 pt-3 space-y-3">
          <h4 class="text-xs font-semibold text-accent uppercase tracking-wider">Tag Restrictions</h4>
          
          <div class="flex flex-col space-y-3">
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-all-tags" ${accessAllTags ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow access to all tags</span>
            </label>
          </div>

          <!-- Tag Selector (Collapsible) -->
          <div id="tag-selector-container" class="${accessAllTags ? 'hidden' : ''} border border-black-300 rounded p-3 bg-black-500 space-y-3">
            <div>
              <label class="block text-[10px] font-semibold text-black-100 uppercase mb-1">Tag Filter Mode</label>
              <select id="perm-tags-not-accessible" class="w-full bg-black-500 text-white px-2 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
                <option value="false" ${!selectedTagsNotAccessible ? 'selected' : ''}>Allow Only Selected Tags</option>
                <option value="true" ${selectedTagsNotAccessible ? 'selected' : ''}>Block Selected Tags</option>
              </select>
            </div>

            <div>
              <p class="text-[10px] font-semibold text-black-100 uppercase mb-1">Select Tags:</p>
              <div class="max-h-28 overflow-y-auto space-y-1">
                ${allTags.length === 0 ? '<p class="text-[11px] text-black-100">No tags available in library</p>' : allTags.map(tag => `
                  <label class="flex items-center space-x-2 text-[11px] cursor-pointer hover:bg-black-400 p-1 rounded">
                    <input type="checkbox" value="${escapeHtml(tag)}" ${itemTagsSelected.includes(tag) ? 'checked' : ''} class="user-tag-checkbox rounded text-accent bg-black-600 border-black-300">
                    <span>${escapeHtml(tag)}</span>
                  </label>
                `).join('')}
              </div>
            </div>
          </div>
        </div>

        <div class="flex justify-end space-x-3 pt-2">
          <button type="button" id="close-user-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold">Cancel</button>
          <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">${isEdit ? 'Save Changes' : 'Create User'}</button>
        </div>
      </form>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  modal.querySelector('#close-user-modal-btn').onclick = closeModal;

  const allLibsCheckbox = modal.querySelector('#perm-all-libraries');
  const libContainer = modal.querySelector('#library-selector-container');
  allLibsCheckbox.onchange = () => {
    if (allLibsCheckbox.checked) {
      libContainer.classList.add('hidden');
    } else {
      libContainer.classList.remove('hidden');
    }
  };

  const allTagsCheckbox = modal.querySelector('#perm-all-tags');
  const tagContainer = modal.querySelector('#tag-selector-container');
  allTagsCheckbox.onchange = () => {
    if (allTagsCheckbox.checked) {
      tagContainer.classList.add('hidden');
    } else {
      tagContainer.classList.remove('hidden');
    }
  };

  const form = modal.querySelector('#user-form');
  form.onsubmit = async (e) => {
    e.preventDefault();

    const username = modal.querySelector('#user-username').value.trim();
    const email = modal.querySelector('#user-email').value.trim();
    const password = modal.querySelector('#user-password').value;
    const type = modal.querySelector('#user-type').value;
    const isActive = modal.querySelector('#user-isactive').checked;

    const download = modal.querySelector('#perm-download').checked;
    const accessExplicitContent = modal.querySelector('#perm-explicit').checked;
    const upload = modal.querySelector('#perm-upload').checked;
    const deleteVal = modal.querySelector('#perm-delete').checked;
    const update = modal.querySelector('#perm-update').checked;
    const accessRss = modal.querySelector('#perm-rss').checked;
    const createShares = modal.querySelector('#perm-shares').checked;
    const accessAllLibraries = allLibsCheckbox.checked;

    const libCheckboxes = modal.querySelectorAll('.user-lib-checkbox:checked');
    const librariesAccessible = Array.from(libCheckboxes).map(cb => cb.value);

    if (!accessAllLibraries && librariesAccessible.length === 0) {
      alert('You must select at least one accessible library if "Access All Libraries" is disabled.');
      return;
    }

    const accessAllTags = allTagsCheckbox.checked;
    const selectedTagsNotAccessible = modal.querySelector('#perm-tags-not-accessible').value === 'true';
    const tagCheckboxes = modal.querySelectorAll('.user-tag-checkbox:checked');
    const itemTagsSelected = Array.from(tagCheckboxes).map(cb => cb.value);

    const permissions = {
      download,
      accessExplicitContent,
      upload,
      delete: deleteVal,
      update,
      accessRss,
      createShares,
      accessAllLibraries,
      accessAllTags,
      selectedTagsNotAccessible,
      itemTagsSelected
    };

    try {
      if (isEdit) {
        const payload = {
          email,
          permissions,
          librariesAccessible,
          itemTagsSelected
        };
        if (password) {
          payload.password = password;
        }
        if (user.type !== 'root') {
          payload.type = type;
          payload.isActive = isActive;
        }
        await request('PATCH', `/api/users/${user.id}`, payload);
      } else {
        const payload = {
          username,
          password,
          email,
          type,
          isActive,
          permissions: {
            ...permissions,
            librariesAccessible,
            accessAllTags,
            itemTagsSelected,
            selectedTagsNotAccessible
          },
          librariesAccessible,
          itemTagsSelected
        };
        await request('POST', '/api/users', payload);
      }
      closeModal();
      if (onSaveSuccess) onSaveSuccess();
    } catch (err) {
      alert(`Failed to save user: ` + err.message);
    }
  };
}

async function renderApiKeysTab() {
  const container = document.getElementById('tab-apikeys');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const [apiKeysResp, users] = await Promise.all([
      request('GET', '/api/api-keys'),
      request('GET', '/api/users')
    ]);
    const apiKeys = apiKeysResp.apiKeys || [];

    container.innerHTML = `
      <div class="space-y-4">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold text-white">API Keys</h3>
          <button id="add-apikey-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">
            + Add API Key
          </button>
        </div>
        
        <div class="border border-black-300 rounded-md bg-primary overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-black-400/60 text-black-100 text-xs uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">Name</th>
                <th class="px-4 py-3">User</th>
                <th class="px-4 py-3">Expires At</th>
                <th class="px-4 py-3">Created At</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="apikeys-list-rows" class="divide-y divide-black-400">
              <!-- Rows will be injected here -->
            </tbody>
          </table>
        </div>
      </div>
    `;

    const addBtn = container.querySelector('#add-apikey-btn');
    addBtn.onclick = () => {
      triggerApiKeyModal(users, () => renderApiKeysTab());
    };

    renderApiKeysListRows(apiKeys, users);
  } catch (err) {
    container.innerHTML = `<div class="text-red-500 text-center py-4">Failed to load API keys: ${escapeHtml(err.message)}</div>`;
  }
}

function renderApiKeysListRows(apiKeys, users) {
  const tbody = document.getElementById('apikeys-list-rows');
  if (!tbody) return;

  tbody.innerHTML = '';

  if (apiKeys.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="5" class="px-4 py-8 text-center text-black-100">
          No API keys generated yet.
        </td>
      </tr>
    `;
    return;
  }

  const userMap = {};
  users.forEach(u => {
    userMap[u.id] = u.username;
  });

  apiKeys.forEach(key => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-black-500/30';

    const expiresAtFormatted = key.expiresAt ? (window.formatDateTime ? window.formatDateTime(key.expiresAt) : key.expiresAt) : 'Never';
    const createdAtFormatted = key.createdAt ? (window.formatDateTime ? window.formatDateTime(key.createdAt) : key.createdAt) : 'Unknown';
    const username = userMap[key.userId] || key.username || 'Unknown';

    tr.innerHTML = `
      <td class="px-4 py-3 font-semibold text-white">${escapeHtml(key.name || '')}</td>
      <td class="px-4 py-3 text-black-50">${escapeHtml(username)}</td>
      <td class="px-4 py-3 text-black-100">${escapeHtml(expiresAtFormatted)}</td>
      <td class="px-4 py-3 text-black-100">${escapeHtml(createdAtFormatted)}</td>
      <td class="px-4 py-3 text-right">
        <button class="delete-apikey-btn text-red-500 hover:text-red-400 font-semibold text-xs" data-id="${key.id}">
          Delete
        </button>
      </td>
    `;

    const deleteBtn = tr.querySelector('.delete-apikey-btn');
    deleteBtn.onclick = async () => {
      const confirmed = confirm(`Are you sure you want to delete the API key "${key.name}"?`);
      if (confirmed) {
        try {
          await request('DELETE', `/api/api-keys/${key.id}`);
          renderApiKeysTab();
        } catch (err) {
          alert(`Failed to delete API key: ${err.message}`);
        }
      }
    };

    tbody.appendChild(tr);
  });
}

function triggerApiKeyModal(users, onSaveSuccess) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 overflow-y-auto';

  const defaultUser = users.find(u => u.type === 'root') || users[0] || { id: '' };

  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-md p-6 rounded-md shadow-lg space-y-4 my-8">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2">Generate API Key</h3>
      
      <form id="apikey-form" class="space-y-4">
        <div>
          <label class="block text-xs text-black-100 mb-1">Key Name</label>
          <input type="text" id="apikey-name" required placeholder="e.g. Home Assistant" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">User</label>
          <select id="apikey-user" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
            ${users.map(u => `<option value="${u.id}" ${u.id === defaultUser.id ? 'selected' : ''}>${escapeHtml(u.username)} (${u.type})</option>`).join('')}
          </select>
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">Expires At (Optional)</label>
          <input type="datetime-local" id="apikey-expires" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div class="flex justify-end space-x-3 pt-2">
          <button type="button" id="close-apikey-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold">Cancel</button>
          <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">Generate</button>
        </div>
      </form>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  modal.querySelector('#close-apikey-modal-btn').onclick = closeModal;

  const form = modal.querySelector('#apikey-form');
  form.onsubmit = async (e) => {
    e.preventDefault();

    const name = modal.querySelector('#apikey-name').value.trim();
    const userId = modal.querySelector('#apikey-user').value;
    const expiresVal = modal.querySelector('#apikey-expires').value;

    let expiresAt = '';
    if (expiresVal) {
      expiresAt = new Date(expiresVal).toISOString();
    }

    try {
      const response = await request('POST', '/api/api-keys', {
        name,
        userId,
        expiresAt
      });
      closeModal();
      if (response && response.apiKey && response.apiKey.token) {
        triggerShowTokenModal(response.apiKey.token, onSaveSuccess);
      } else {
        if (onSaveSuccess) onSaveSuccess();
      }
    } catch (err) {
      alert(`Failed to create API key: ${err.message}`);
    }
  };
}

function triggerShowTokenModal(token, onClosed) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 overflow-y-auto';

  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-md p-6 rounded-md shadow-lg space-y-4 my-8">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2 text-accent">API Key Generated</h3>
      
      <div class="space-y-4">
        <p class="text-sm text-black-50">
          Make sure to copy your API key now. You won't be able to see it again!
        </p>

        <div class="bg-black-500 p-3 rounded border border-black-300 flex items-center justify-between">
          <input type="text" readonly value="${escapeHtml(token)}" class="bg-transparent text-white font-mono text-xs w-full focus:outline-none select-all mr-2" id="generated-token-input">
          <button id="copy-token-btn" class="text-accent hover:text-white font-semibold text-xs shrink-0">
            Copy
          </button>
        </div>

        <div class="flex justify-end pt-2">
          <button type="button" id="close-token-modal-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">Done</button>
        </div>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const copyBtn = modal.querySelector('#copy-token-btn');
  const tokenInput = modal.querySelector('#generated-token-input');
  copyBtn.onclick = () => {
    tokenInput.select();
    try {
      document.execCommand('copy');
      copyBtn.textContent = 'Copied!';
      setTimeout(() => {
        copyBtn.textContent = 'Copy';
      }, 2000);
    } catch (err) {
      console.error('Failed to copy token: ', err);
    }
  };

  const closeModal = () => {
    modal.remove();
    if (onClosed) onClosed();
  };

  modal.querySelector('#close-token-modal-btn').onclick = closeModal;
}

// Render the Listening Sessions settings tab
async function renderListeningSessionsTab() {
  const container = document.getElementById('tab-listening-sessions');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const [users, sessionsResp] = await Promise.all([
      request('GET', '/api/users'),
      request('GET', '/api/playback-sessions')
    ]);
    currentSessions = sessionsResp.sessions || [];
    selectedUserIdFilter = '';

    container.innerHTML = `
      <div class="space-y-4">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold text-white">Listening Sessions</h3>
          <div class="flex items-center space-x-2">
            <label for="filter-session-user" class="text-xs text-black-100 uppercase tracking-wider">Filter by User:</label>
            <select id="filter-session-user" class="bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
              <option value="">All Users</option>
              ${users.map(u => `<option value="${u.id}">${escapeHtml(u.username)}</option>`).join('')}
            </select>
          </div>
        </div>

        <div class="border border-black-300 rounded-md bg-primary overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-black-400/60 text-black-100 text-xs uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">User</th>
                <th class="px-4 py-3">Item</th>
                <th class="px-4 py-3">Play Method</th>
                <th class="px-4 py-3">Device Info</th>
                <th class="px-4 py-3">Time Listened</th>
                <th class="px-4 py-3">Last Position/Last Time</th>
                <th class="px-4 py-3">Last Updated</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="sessions-list-rows" class="divide-y divide-black-400">
              <!-- Rows will be injected here -->
            </tbody>
          </table>
        </div>
      </div>
    `;

    const select = container.querySelector('#filter-session-user');
    select.onchange = () => {
      selectedUserIdFilter = select.value;
      renderListeningSessionsListRows(getFilteredSessions());
    };

    renderListeningSessionsListRows(getFilteredSessions());
  } catch (err) {
    container.innerHTML = `<div class="text-red-500 text-center py-4">Failed to load listening sessions: ${escapeHtml(err.message)}</div>`;
  }
}

function renderListeningSessionsListRows(sessions) {
  const tbody = document.getElementById('sessions-list-rows');
  if (!tbody) return;

  tbody.innerHTML = '';

  if (sessions.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="8" class="px-4 py-8 text-center text-black-100">
          No listening sessions found.
        </td>
      </tr>
    `;
    return;
  }

  sessions.forEach(session => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-black-500/30';

    const timeListenedFormatted = formatSessionTime(session.timeListened);
    const lastTimeFormatted = formatSessionTime(session.lastTime);
    const updatedAtFormatted = session.updatedAt ? (window.formatDateTime ? window.formatDateTime(session.updatedAt) : session.updatedAt) : 'Unknown';

    // Verify current user permissions to show Close button
    const curUser = window.currentUser || {};
    const canClose = curUser.type === 'root' || curUser.type === 'admin' || curUser.id === session.userId;

    let actionsHtml = '';
    if (canClose) {
      actionsHtml = `
        <button class="close-session-btn text-red-500 hover:text-red-400 font-semibold text-xs transition-colors duration-150" data-id="${session.id}">
          Close Session
        </button>
      `;
    }

    tr.innerHTML = `
      <td class="px-4 py-3 font-semibold text-white">${escapeHtml(session.username || 'Unknown')}</td>
      <td class="px-4 py-3 text-black-50 font-medium">${escapeHtml(session.title || 'Unknown')}</td>
      <td class="px-4 py-3 text-black-100"><span class="px-2 py-0.5 rounded text-xs bg-black-400 font-mono">${escapeHtml(session.playMethod || 'HLS')}</span></td>
      <td class="px-4 py-3 text-black-100">${escapeHtml(session.deviceInfo || 'Web Client')}</td>
      <td class="px-4 py-3 text-black-100 font-mono text-xs">${escapeHtml(timeListenedFormatted)}</td>
      <td class="px-4 py-3 text-black-100 font-mono text-xs">${escapeHtml(lastTimeFormatted)}</td>
      <td class="px-4 py-3 text-black-100">${escapeHtml(updatedAtFormatted)}</td>
      <td class="px-4 py-3 text-right">${actionsHtml}</td>
    `;

    if (canClose) {
      const closeBtn = tr.querySelector('.close-session-btn');
      closeBtn.onclick = async () => {
        if (confirm(`Are you sure you want to close this playback session for ${session.username || 'user'}?`)) {
          try {
            await request('DELETE', `/api/playback-sessions/${session.id}`);
            // Note: socket listener will automatically remove it and re-render.
            // But we can also remove it locally right away for an instant UI update:
            currentSessions = currentSessions.filter(s => s.id !== session.id);
            renderListeningSessionsListRows(getFilteredSessions());
          } catch (err) {
            alert('Failed to close playback session: ' + err.message);
          }
        }
      };
    }

    tbody.appendChild(tr);
  });
}

function formatSessionTime(secs) {
  if (isNaN(secs) || secs === Infinity || secs === null || secs === undefined) return '0:00';
  const hours = Math.floor(secs / 3600);
  const minutes = Math.floor((secs % 3600) / 60);
  const seconds = Math.floor(secs % 60);
  
  const formattedSeconds = seconds < 10 ? `0${seconds}` : seconds;
  if (hours > 0) {
    const formattedMinutes = minutes < 10 ? `0${minutes}` : minutes;
    return `${hours}:${formattedMinutes}:${formattedSeconds}`;
  }
  return `${minutes}:${formattedSeconds}`;
}

/**
 * Render the Logs tab.
 * Displays server logs with levels and content filtering, and provides level configuration.
 */
async function renderLogsTab() {
  const container = document.getElementById('tab-logs');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    // 1. Fetch settings and logs concurrently
    const [settings, loggerData] = await Promise.all([
      request('GET', '/api/settings'),
      request('GET', '/api/logger-data')
    ]);

    // logLevel mapping: DEBUG = 1, INFO = 2, WARN = 3, ERROR = 4 (default to INFO=2)
    const activeLogLevel = (settings && settings.logLevel !== undefined) ? parseInt(settings.logLevel, 10) : 2;
    const logEntries = (loggerData && loggerData.currentDailyLogs) || [];

    // 2. Render HTML Layout structure with search filter & dropdown level selector
    container.innerHTML = `
      <div class="bg-primary border border-black-300 p-6 rounded-md space-y-6">
        <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4 border-b border-black-400 pb-4">
          <div>
            <h3 class="text-lg font-semibold text-white">Server Logs</h3>
            <p class="text-xs text-black-100 mt-1">View system logs and adjust log verbosity.</p>
          </div>
          <div class="flex flex-col sm:flex-row sm:items-center gap-3">
            <!-- Log Level Selector -->
            <div class="flex items-center space-x-2">
              <label for="log-level-select" class="text-xs text-black-100 uppercase tracking-wider whitespace-nowrap">Log Level:</label>
              <select id="log-level-select" class="bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
                <option value="1" ${activeLogLevel === 1 ? 'selected' : ''}>DEBUG</option>
                <option value="2" ${activeLogLevel === 2 ? 'selected' : ''}>INFO</option>
                <option value="3" ${activeLogLevel === 3 ? 'selected' : ''}>WARN</option>
                <option value="4" ${activeLogLevel === 4 ? 'selected' : ''}>ERROR</option>
              </select>
            </div>
            
            <!-- Search Input -->
            <div class="flex items-center space-x-2">
              <input type="text" id="log-search-input" placeholder="Search logs..." class="bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm w-full sm:w-48">
            </div>
          </div>
        </div>

        <!-- Logs Console/Viewer Panel -->
        <div id="logs-console" class="bg-black-900 border border-black-400 rounded p-4 font-mono text-xs overflow-y-auto h-[500px] space-y-1 text-black-50 scrollbar-thin">
          <!-- Log lines will be injected here -->
        </div>
      </div>
    `;

    const logConsole = container.querySelector('#logs-console');
    const logLevelSelect = container.querySelector('#log-level-select');
    const logSearchInput = container.querySelector('#log-search-input');

    let currentSelectedLevel = parseInt(logLevelSelect.value, 10);
    let currentSearchQuery = '';

    // 3. Render helper displaying logs based on selected level and search query filters
    function displayLogs() {
      logConsole.innerHTML = '';
      
      const filtered = logEntries.filter(entry => {
        // Level filter: log entry level >= currentSelectedLevel
        const matchesLevel = entry.level >= currentSelectedLevel;
        
        // Search text query filter: matches message, timestamp, or levelName
        const query = currentSearchQuery.trim().toLowerCase();
        const entryMsg = String(entry.message || '').toLowerCase();
        const entryTimestamp = String(entry.timestamp || '').toLowerCase();
        const entryLevelName = String(entry.levelName || '').toLowerCase();

        const matchesSearch = !query || 
          entryMsg.includes(query) ||
          entryTimestamp.includes(query) ||
          entryLevelName.includes(query);
          
        return matchesLevel && matchesSearch;
      });

      if (filtered.length === 0) {
        logConsole.innerHTML = `<div class="text-black-100 text-center py-4">No matching logs found.</div>`;
        return;
      }

      const limitedLogs = filtered.slice(-1000); // Limit to last 1000 matches
      const fragment = document.createDocumentFragment();
      limitedLogs.forEach(entry => {
        const div = document.createElement('div');
        div.className = 'whitespace-pre-wrap leading-relaxed py-0.5 border-b border-black-400/10 last:border-b-0';
        
        // Apply level-specific console styling
        let levelColorClass = 'text-black-100'; // Default DEBUG (gray)
        if (entry.level === 2) {
          levelColorClass = 'text-green-400'; // INFO (green)
        } else if (entry.level === 3) {
          levelColorClass = 'text-yellow-400'; // WARN (yellow)
        } else if (entry.level === 4) {
          levelColorClass = 'text-red-400'; // ERROR (red)
        }

        const timestamp = escapeHtml(String(entry.timestamp || ''));
        const levelName = escapeHtml(String(entry.levelName || 'INFO'));
        const message = escapeHtml(String(entry.message || ''));

        div.innerHTML = `<span class="text-black-100">[${timestamp}]</span> <span class="${levelColorClass} font-semibold">[${levelName}]</span> <span class="text-white">${message}</span>`;
        fragment.appendChild(div);
      });
      logConsole.appendChild(fragment);

      // Automatically scroll to the bottom of the console container
      logConsole.scrollTop = logConsole.scrollHeight;
    }

    // Subscribe to real-time socket logs
    sendEvent('set_log_listener', activeLogLevel);

    const logSocketCallback = (logMsg) => {
      logEntries.push(logMsg);
      if (logEntries.length > 2000) {
        logEntries.shift();
      }
      displayLogs();
    };

    onEvent('log', logSocketCallback);

    // Setup cleanup function on global window object
    window.cleanupSettings = () => {
      sendEvent('remove_log_listener');
      offEvent('log', logSocketCallback);
    };

    // Initial log display
    displayLogs();

    // 4. Setup Event Listeners
    logLevelSelect.onchange = async () => {
      const val = parseInt(logLevelSelect.value, 10);
      const prevVal = currentSelectedLevel;
      currentSelectedLevel = val;
      displayLogs();

      // Update socket listener level
      sendEvent('set_log_listener', val);

      try {
        await request('PATCH', '/api/settings', { logLevel: val });
      } catch (err) {
        alert('Failed to save log level on server: ' + err.message);
        currentSelectedLevel = prevVal;
        logLevelSelect.value = prevVal;
        sendEvent('set_log_listener', prevVal);
        displayLogs();
      }
    };

    logSearchInput.oninput = () => {
      currentSearchQuery = logSearchInput.value;
      displayLogs();
    };

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm">Failed to load logs: ${escapeHtml(err.message)}</p>`;
  }
}

export async function renderNotificationsTab() {
  const container = document.getElementById('tab-notifications');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const settings = await request('GET', '/api/notifications');
    const appriseApiUrl = settings.appriseApiUrl || '';
    const maxNotificationQueue = (settings.maxNotificationQueue !== undefined && settings.maxNotificationQueue !== null) ? settings.maxNotificationQueue : 25;
    const maxFailedAttempts = (settings.maxFailedAttempts !== undefined && settings.maxFailedAttempts !== null) ? settings.maxFailedAttempts : 5;
    const notifications = settings.notifications || [];

    container.innerHTML = `
      <form id="apprise-settings-form" class="space-y-6 bg-primary border border-black-300 p-6 rounded-md">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Apprise Notification Settings</h3>
        
        <div class="space-y-4">
          <div>
            <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Apprise API URL</label>
            <input type="text" id="apprise-api-url" value="${escapeHtml(appriseApiUrl)}" placeholder="e.g. http://localhost:8000" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            <p class="text-xs text-black-100 mt-1">The URL of your running Apprise API instance.</p>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Max Notification Queue</label>
              <input type="number" id="max-notification-queue" value="${maxNotificationQueue}" min="1" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
              <p class="text-xs text-black-100 mt-1">Maximum size of the notification queue.</p>
            </div>

            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Max Failed Attempts</label>
              <input type="number" id="max-failed-attempts" value="${maxFailedAttempts}" min="1" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
              <p class="text-xs text-black-100 mt-1">Maximum failed attempts before pruning notification.</p>
            </div>
          </div>
        </div>

        <button type="submit" id="save-apprise-settings-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity">Save General Settings</button>
      </form>

      <hr class="border-black-400">

      <div class="space-y-4">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold text-white">Notification Setups</h3>
          <button id="add-notification-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">
            + Create
          </button>
        </div>
        
        <div class="border border-black-300 rounded-md bg-primary overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-black-400/60 text-black-100 text-xs uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">ID</th>
                <th class="px-4 py-3">Event</th>
                <th class="px-4 py-3">Enabled</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="notifications-list-rows" class="divide-y divide-black-400">
              <!-- Rows will be injected dynamically -->
            </tbody>
          </table>
        </div>
      </div>
    `;

    renderNotificationsListRows(notifications, settings);

    document.getElementById('apprise-settings-form').onsubmit = async (e) => {
      e.preventDefault();
      try {
        const appriseUrlInput = document.getElementById('apprise-api-url').value.trim();
        const maxQueueVal = parseInt(document.getElementById('max-notification-queue').value, 10);
        const maxFailedVal = parseInt(document.getElementById('max-failed-attempts').value, 10);

        const payload = {
          appriseApiUrl: appriseUrlInput || null,
          maxNotificationQueue: isNaN(maxQueueVal) ? 25 : maxQueueVal,
          maxFailedAttempts: isNaN(maxFailedVal) ? 5 : maxFailedVal,
          notifications: notifications
        };

        await request('POST', '/api/notifications', payload);
        alert('Notification settings saved successfully!');
        renderNotificationsTab();
      } catch (err) {
        alert('Failed to save settings: ' + err.message);
      }
    };

    document.getElementById('add-notification-btn').onclick = () => {
      triggerCreateNotificationModal(settings, () => {
        renderNotificationsTab();
      });
    };

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm">Failed to load notifications settings: ${escapeHtml(err.message)}</p>`;
  }
}

function renderNotificationsListRows(notifications, allSettings) {
  const tbody = document.getElementById('notifications-list-rows');
  if (!tbody) return;

  tbody.innerHTML = '';

  if (!notifications || notifications.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="4" class="px-4 py-8 text-center text-black-100">
          No notification setups created yet.
        </td>
      </tr>
    `;
    return;
  }

  notifications.forEach(notif => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-black-500/30';

    const enabledBadge = notif.enabled 
      ? `<span class="bg-green-900/50 text-green-200 text-[10px] px-1.5 py-0.5 rounded font-normal uppercase">Enabled</span>`
      : `<span class="bg-red-900/50 text-red-200 text-[10px] px-1.5 py-0.5 rounded font-normal uppercase">Disabled</span>`;

    tr.innerHTML = `
      <td class="px-4 py-3 font-mono text-xs text-white">${escapeHtml(notif.id || '')}</td>
      <td class="px-4 py-3 text-black-50">${escapeHtml(notif.eventName || '')}</td>
      <td class="px-4 py-3">${enabledBadge}</td>
      <td class="px-4 py-3 text-right">
        <button class="delete-notif-btn text-red-500 hover:text-red-400 font-semibold text-xs" data-id="${notif.id}">
          Delete
        </button>
      </td>
    `;

    const deleteBtn = tr.querySelector('.delete-notif-btn');
    deleteBtn.onclick = async () => {
      if (confirm('Are you sure you want to delete this notification setup?')) {
        try {
          const appriseUrlInput = document.getElementById('apprise-api-url') ? document.getElementById('apprise-api-url').value.trim() : allSettings.appriseApiUrl;
          const maxQueueVal = document.getElementById('max-notification-queue') ? parseInt(document.getElementById('max-notification-queue').value, 10) : allSettings.maxNotificationQueue;
          const maxFailedVal = document.getElementById('max-failed-attempts') ? parseInt(document.getElementById('max-failed-attempts').value, 10) : allSettings.maxFailedAttempts;

          const updatedNotifications = (allSettings.notifications || []).filter(n => n.id !== notif.id);
          const payload = {
            appriseApiUrl: appriseUrlInput || null,
            maxNotificationQueue: isNaN(maxQueueVal) ? 25 : maxQueueVal,
            maxFailedAttempts: isNaN(maxFailedVal) ? 5 : maxFailedVal,
            notifications: updatedNotifications
          };
          await request('POST', '/api/notifications', payload);
          renderNotificationsTab();
        } catch (err) {
          alert('Failed to delete notification setup: ' + err.message);
        }
      }
    };

    tbody.appendChild(tr);
  });
}

function triggerCreateNotificationModal(allSettings, onSaveSuccess) {
  const libraries = getLibrariesList() || [];

  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 overflow-y-auto';

  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-md p-6 rounded-md shadow-lg space-y-4 my-8">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2">Create Notification Setup</h3>
      
      <form id="notification-form" class="space-y-4">
        <div>
          <label class="block text-xs text-black-100 mb-1">Event</label>
          <select id="notif-eventName" required class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
            <option value="onPodcastEpisodeDownloaded">onPodcastEpisodeDownloaded (Podcast Episode Downloaded)</option>
            <option value="onBackupCompleted">onBackupCompleted (Backup Completed)</option>
            <option value="onBackupFailed">onBackupFailed (Backup Failed)</option>
            <option value="onTest">onTest (Test Notification)</option>
          </select>
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">Library (Optional)</label>
          <select id="notif-libraryId" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
            <option value="">All Libraries</option>
            ${libraries.map(lib => `<option value="${lib.id}">${escapeHtml(lib.name)}</option>`).join('')}
          </select>
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">Apprise URLs (comma or newline separated)</label>
          <textarea id="notif-urls" required rows="3" placeholder="e.g. mailto://user:pass@gmail.com, discord://webhook_id/webhook_token" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm"></textarea>
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">Title Template</label>
          <input type="text" id="notif-titleTemplate" required placeholder="e.g. New {{podcastTitle}} Episode!" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">Body Template</label>
          <textarea id="notif-bodyTemplate" required rows="2" placeholder="e.g. {{episodeTitle}} added to {{libraryName}}." class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm"></textarea>
        </div>

        <div class="flex items-center pb-2">
          <label class="flex items-center space-x-3 cursor-pointer text-sm">
            <span class="abs-switch">
              <input type="checkbox" id="notif-enabled" checked>
              <span class="abs-slider"></span>
            </span>
            <span>Enabled</span>
          </label>
        </div>

        <div class="flex justify-end space-x-3 pt-2">
          <button type="button" id="close-notif-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold">Cancel</button>
          <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">Create</button>
        </div>
      </form>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  modal.querySelector('#close-notif-modal-btn').onclick = closeModal;

  const form = modal.querySelector('#notification-form');
  form.onsubmit = async (e) => {
    e.preventDefault();

    const eventName = modal.querySelector('#notif-eventName').value;
    const libraryIdVal = modal.querySelector('#notif-libraryId').value;
    const libraryId = libraryIdVal ? libraryIdVal : null;
    const urlsVal = modal.querySelector('#notif-urls').value;
    const urls = urlsVal.split(/[\n,]+/).map(u => u.trim()).filter(Boolean);
    const titleTemplate = modal.querySelector('#notif-titleTemplate').value.trim();
    const bodyTemplate = modal.querySelector('#notif-bodyTemplate').value.trim();
    const enabled = modal.querySelector('#notif-enabled').checked;

    const newNotif = {
      id: 'notif_' + Math.floor(Math.random() * 16777215).toString(16),
      libraryId,
      eventName,
      urls,
      titleTemplate,
      bodyTemplate,
      enabled
    };

    try {
      const appriseUrlInput = document.getElementById('apprise-api-url') ? document.getElementById('apprise-api-url').value.trim() : allSettings.appriseApiUrl;
      const maxQueueVal = document.getElementById('max-notification-queue') ? parseInt(document.getElementById('max-notification-queue').value, 10) : allSettings.maxNotificationQueue;
      const maxFailedVal = document.getElementById('max-failed-attempts') ? parseInt(document.getElementById('max-failed-attempts').value, 10) : allSettings.maxFailedAttempts;

      const updatedNotifications = [...(allSettings.notifications || []), newNotif];
      const payload = {
        appriseApiUrl: appriseUrlInput || null,
        maxNotificationQueue: isNaN(maxQueueVal) ? 25 : maxQueueVal,
        maxFailedAttempts: isNaN(maxFailedVal) ? 5 : maxFailedVal,
        notifications: updatedNotifications
      };

      await request('POST', '/api/notifications', payload);
      closeModal();
      if (onSaveSuccess) onSaveSuccess();
    } catch (err) {
      alert(`Failed to save notification setup: ${err.message}`);
    }
  };
}

async function renderFeedsTab() {
  const container = document.getElementById('tab-feeds');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const feedsResp = await request('GET', '/api/feeds');
    const feeds = feedsResp.feeds || [];

    container.innerHTML = `
      <div class="space-y-4">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold text-white">Active RSS Feeds</h3>
          <button id="settings-opml-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-3 py-1.5 rounded text-xs transition-colors flex items-center gap-1">
            <span class="material-symbols text-sm">import_export</span>
            OPML Import/Export
          </button>
        </div>
        
        <div class="border border-black-300 rounded-md bg-primary overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-black-400/60 text-black-100 text-xs uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">Title / Entity</th>
                <th class="px-4 py-3">Type</th>
                <th class="px-4 py-3">RSS Feed URL</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="feeds-list-rows" class="divide-y divide-black-400">
              <!-- Rows will be injected here -->
            </tbody>
          </table>
          ${feeds.length === 0 ? `<div class="p-8 text-center text-black-100">No active RSS feeds. You can open a feed from any item's details view.</div>` : ''}
        </div>
      </div>
    `;

    const settingsOpmlBtn = container.querySelector('#settings-opml-btn');
    if (settingsOpmlBtn) {
      settingsOpmlBtn.onclick = () => {
        const activeLibId = getActiveLibraryId();
        const libs = getLibrariesList() || [];
        let targetLibId = activeLibId;
        
        const activeLib = libs.find(l => l.id === activeLibId);
        if (!activeLib || activeLib.mediaType !== 'podcast') {
          const firstPodcastLib = libs.find(l => l.mediaType === 'podcast');
          if (firstPodcastLib) {
            targetLibId = firstPodcastLib.id;
          } else {
            alert('Please create a Podcast library first to import/export OPML.');
            return;
          }
        }

        import('./opml.js').then(module => {
          module.openOPMLModal(targetLibId);
        });
      };
    }

    const listRows = document.getElementById('feeds-list-rows');
    if (!listRows) return;

    feeds.forEach(feed => {
      const tr = document.createElement('tr');
      tr.className = 'hover:bg-black-500/30';
      tr.innerHTML = `
        <td class="px-4 py-3 font-medium text-white">${escapeHtml(feed.title || feed.entityId)}</td>
        <td class="px-4 py-3 text-black-50 uppercase text-xs">${escapeHtml(feed.type)}</td>
        <td class="px-4 py-3 text-black-100">
          <div class="flex items-center gap-2">
            <span class="truncate max-w-xs font-mono text-xs select-all">${escapeHtml(feed.feedUrl)}</span>
            <button class="copy-feed-btn text-accent hover:underline text-xs" data-url="${escapeHtml(feed.feedUrl)}">
              Copy
            </button>
          </div>
        </td>
        <td class="px-4 py-3 text-right">
          <button class="delete-feed-btn text-error hover:underline text-xs" data-id="${escapeHtml(feed.id)}">
            Close Feed
          </button>
        </td>
      `;

      // Wire up copy button
      tr.querySelector('.copy-feed-btn').onclick = (e) => {
        const btn = e.target;
        navigator.clipboard.writeText(btn.dataset.url).then(() => {
          const oldText = btn.textContent;
          btn.textContent = 'Copied!';
          setTimeout(() => { btn.textContent = oldText; }, 2000);
        });
      };

      // Wire up delete button
      tr.querySelector('.delete-feed-btn').onclick = async (e) => {
        if (!confirm('Are you sure you want to close this RSS feed?')) return;
        const btn = e.target;
        try {
          await request('DELETE', `/api/feeds/${btn.dataset.id}`);
          renderFeedsTab();
        } catch (err) {
          alert('Failed to close feed: ' + err.message);
        }
      };

      listRows.appendChild(tr);
    });
  } catch (err) {
    container.innerHTML = `
      <div class="text-error p-4 text-center">
        Failed to load active feeds: ${escapeHtml(err.message)}
      </div>
    `;
  }
}

export async function renderEmailsTab() {
  const container = document.getElementById('tab-emails');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const settings = await request('GET', '/api/emails/settings');
    const host = settings.host || '';
    const port = settings.port || 587;
    const secure = settings.secure || false;
    const rejectUnauthorized = settings.rejectUnauthorized !== false;
    const user = settings.user || '';
    const pass = settings.pass || '';
    const fromAddress = settings.fromAddress || '';
    const testAddress = settings.testAddress || '';
    const devices = settings.ereaderDevices || [];

    // Fetch all users to display names in the user selector modal
    const users = await request('GET', '/api/users');

    container.innerHTML = `
      <form id="email-settings-form" class="space-y-6 bg-primary border border-black-300 p-6 rounded-md">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">SMTP Configuration</h3>
        
        <div class="space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">SMTP Host</label>
              <input type="text" id="email-host" value="${escapeHtml(host)}" placeholder="e.g. smtp.gmail.com" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>

            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">SMTP Port</label>
              <input type="number" id="email-port" value="${port}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">SMTP Username</label>
              <input type="text" id="email-user" value="${escapeHtml(user)}" placeholder="Username or email" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>

            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">SMTP Password</label>
              <input type="password" id="email-pass" value="${escapeHtml(pass)}" placeholder="Password" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">From Address</label>
              <input type="text" id="email-from" value="${escapeHtml(fromAddress)}" placeholder="e.g. library@my-domain.com" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>

            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Test Recipient Address</label>
              <input type="text" id="email-test-addr" value="${escapeHtml(testAddress)}" placeholder="e.g. my-kindle@kindle.com" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
          </div>

          <div class="flex flex-col space-y-3 pt-2">
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="email-secure" ${secure ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Secure connection (SSL/TLS)</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="email-reject-unauthorized" ${rejectUnauthorized ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Reject unauthorized SSL certificates</span>
            </label>
          </div>
        </div>

        <div class="flex space-x-4 pt-2">
          <button type="submit" id="save-email-settings-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity">Save Settings</button>
          <button type="button" id="test-email-settings-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-white font-bold px-4 py-2 rounded transition-colors">Send Test Email</button>
        </div>
      </form>

      <hr class="border-black-400">

      <div class="space-y-4">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold text-white">E-Reader Devices</h3>
          <button id="add-ereader-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">
            + Add Device
          </button>
        </div>
        
        <div class="border border-black-300 rounded-md bg-primary overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-black-400/60 text-black-100 text-xs uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">Device Name</th>
                <th class="px-4 py-3">Device Email</th>
                <th class="px-4 py-3">Availability</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="ereader-list-rows" class="divide-y divide-black-400">
              <!-- Rows will be injected dynamically -->
            </tbody>
          </table>
        </div>
      </div>
    `;

    renderEreaderDevicesRows(devices, users, settings);

    // Save Email Settings Handler
    document.getElementById('email-settings-form').onsubmit = async (e) => {
      e.preventDefault();
      try {
        const hostVal = document.getElementById('email-host').value.trim();
        const portVal = parseInt(document.getElementById('email-port').value, 10);
        const userVal = document.getElementById('email-user').value.trim();
        const passVal = document.getElementById('email-pass').value;
        const fromVal = document.getElementById('email-from').value.trim();
        const testVal = document.getElementById('email-test-addr').value.trim();
        const secureVal = document.getElementById('email-secure').checked;
        const rejectVal = document.getElementById('email-reject-unauthorized').checked;

        const payload = {
          host: hostVal,
          port: isNaN(portVal) ? 587 : portVal,
          secure: secureVal,
          rejectUnauthorized: rejectVal,
          user: userVal,
          pass: passVal,
          fromAddress: fromVal,
          testAddress: testVal
        };

        await request('PATCH', '/api/emails/settings', payload);
        alert('SMTP configuration saved successfully!');
        renderEmailsTab();
      } catch (err) {
        alert('Failed to save configuration: ' + err.message);
      }
    };

    // Test Email Handler
    document.getElementById('test-email-settings-btn').onclick = async () => {
      try {
        const hostVal = document.getElementById('email-host').value.trim();
        const portVal = parseInt(document.getElementById('email-port').value, 10);
        const userVal = document.getElementById('email-user').value.trim();
        const passVal = document.getElementById('email-pass').value;
        const fromVal = document.getElementById('email-from').value.trim();
        const testVal = document.getElementById('email-test-addr').value.trim();
        const secureVal = document.getElementById('email-secure').checked;
        const rejectVal = document.getElementById('email-reject-unauthorized').checked;

        const payload = {
          host: hostVal,
          port: isNaN(portVal) ? 587 : portVal,
          secure: secureVal,
          rejectUnauthorized: rejectVal,
          user: userVal,
          pass: passVal,
          fromAddress: fromVal,
          testAddress: testVal
        };

        if (!payload.testAddress) {
          alert('Please specify a Test Recipient Address');
          return;
        }

        const btn = document.getElementById('test-email-settings-btn');
        btn.textContent = 'Sending...';
        btn.disabled = true;

        try {
          await request('POST', '/api/emails/test', payload);
          alert('Test email sent successfully! Please check the recipient address.');
        } finally {
          btn.textContent = 'Send Test Email';
          btn.disabled = false;
        }
      } catch (err) {
        alert('Failed to send test email: ' + err.message);
      }
    };

    // Add Ereader Device Handler
    document.getElementById('add-ereader-btn').onclick = () => {
      triggerEreaderDeviceModal(null, devices, users, settings);
    };

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm">Failed to load email settings: ${escapeHtml(err.message)}</p>`;
  }
}

function renderEreaderDevicesRows(devices, users, settings) {
  const rowsContainer = document.getElementById('ereader-list-rows');
  if (!rowsContainer) return;

  if (devices.length === 0) {
    rowsContainer.innerHTML = `
      <tr>
        <td colspan="4" class="px-4 py-8 text-center text-black-100 text-sm">No e-reader devices configured.</td>
      </tr>
    `;
    return;
  }

  rowsContainer.innerHTML = '';
  devices.forEach((dev, idx) => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-black-500/30 transition-colors';

    let availText = '';
    if (dev.availabilityOption === 'allUsers') {
      availText = '<span class="text-accent">All Users</span>';
    } else if (dev.availabilityOption === 'adminOrUp') {
      availText = '<span class="text-yellow-400">Admin or Up</span>';
    } else if (dev.availabilityOption === 'specificUsers') {
      const allowedNames = (dev.users || []).map(uId => {
        const u = users.find(x => x.id === uId);
        return u ? u.username : uId;
      }).join(', ');
      availText = `<span class="text-blue-400 font-semibold">Specific Users</span> <span class="text-xs text-black-100">(${escapeHtml(allowedNames || 'none')})</span>`;
    }

    tr.innerHTML = `
      <td class="px-4 py-3 font-semibold text-white">${escapeHtml(dev.name)}</td>
      <td class="px-4 py-3 font-mono text-black-50">${escapeHtml(dev.email)}</td>
      <td class="px-4 py-3 text-sm">${availText}</td>
      <td class="px-4 py-3 text-right space-x-2">
        <button class="edit-dev-btn text-accent hover:underline text-xs font-semibold" data-index="${idx}">Edit</button>
        <button class="delete-dev-btn text-red-400 hover:underline text-xs font-semibold" data-index="${idx}">Delete</button>
      </td>
    `;

    tr.querySelector('.edit-dev-btn').onclick = (e) => {
      const index = parseInt(e.target.dataset.index, 10);
      triggerEreaderDeviceModal(devices[index], devices, users, settings);
    };

    tr.querySelector('.delete-dev-btn').onclick = async (e) => {
      const index = parseInt(e.target.dataset.index, 10);
      if (!confirm(`Are you sure you want to delete device "${devices[index].name}"?`)) return;

      const updatedDevices = devices.filter((_, i) => i !== index);
      try {
        await request('POST', '/api/emails/ereader-devices', { ereaderDevices: updatedDevices });
        alert('Device deleted successfully.');
        renderEmailsTab();
      } catch (err) {
        alert('Failed to delete device: ' + err.message);
      }
    };

    rowsContainer.appendChild(tr);
  });
}

function triggerEreaderDeviceModal(device = null, devices, users, settings) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 overflow-y-auto';

  const isEdit = !!device;
  const devName = isEdit ? device.name : '';
  const devEmail = isEdit ? device.email : '';
  const availOption = isEdit ? device.availabilityOption : 'allUsers';
  const selectedUsers = isEdit ? (device.users || []) : [];

  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-lg rounded-md shadow-2xl p-6 relative">
      <h3 class="text-lg font-bold text-white mb-4 border-b border-black-400 pb-2">
        ${isEdit ? 'Edit E-Reader Device' : 'Add E-Reader Device'}
      </h3>
      
      <form id="ereader-device-form" class="space-y-4 text-left">
        <div>
          <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Device Name</label>
          <input type="text" id="dev-name" value="${escapeHtml(devName)}" required placeholder="e.g. My Kindle" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
        </div>

        <div>
          <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Device Email Address</label>
          <input type="email" id="dev-email" value="${escapeHtml(devEmail)}" required placeholder="e.g. name@kindle.com" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
        </div>

        <div>
          <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Availability Option</label>
          <select id="dev-availability" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            <option value="allUsers" ${availOption === 'allUsers' ? 'selected' : ''}>All Users</option>
            <option value="adminOrUp" ${availOption === 'adminOrUp' ? 'selected' : ''}>Admin or Up</option>
            <option value="specificUsers" ${availOption === 'specificUsers' ? 'selected' : ''}>Specific Users</option>
          </select>
        </div>

        <div id="dev-users-selection-container" class="hidden space-y-2 border border-black-400 p-3 rounded-md bg-black-500/20 max-h-40 overflow-y-auto">
          <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1">Select Allowed Users</label>
          <div class="space-y-1">
            ${users.map(u => `
              <label class="flex items-center space-x-2 cursor-pointer text-sm">
                <input type="checkbox" name="dev-allowed-users" value="${u.id}" ${selectedUsers.includes(u.id) ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
                <span>${escapeHtml(u.username)}</span>
              </label>
            `).join('')}
          </div>
        </div>

        <div class="flex justify-end space-x-3 pt-4 border-t border-black-400">
          <button type="button" id="close-ereader-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold">Cancel</button>
          <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">Save</button>
        </div>
      </form>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  modal.querySelector('#close-ereader-modal-btn').onclick = closeModal;

  const availabilitySelect = modal.querySelector('#dev-availability');
  const usersContainer = modal.querySelector('#dev-users-selection-container');

  const toggleUsersContainer = () => {
    if (availabilitySelect.value === 'specificUsers') {
      usersContainer.classList.remove('hidden');
    } else {
      usersContainer.classList.add('hidden');
    }
  };

  availabilitySelect.onchange = toggleUsersContainer;
  toggleUsersContainer(); // Initial run

  const form = modal.querySelector('#ereader-device-form');
  form.onsubmit = async (e) => {
    e.preventDefault();

    const nameVal = modal.querySelector('#dev-name').value.trim();
    const emailVal = modal.querySelector('#dev-email').value.trim();
    const availVal = availabilitySelect.value;

    let usersVal = [];
    if (availVal === 'specificUsers') {
      const checkedBoxes = modal.querySelectorAll('input[name="dev-allowed-users"]:checked');
      checkedBoxes.forEach(cb => usersVal.push(cb.value));
    }

    const payloadDevice = {
      name: nameVal,
      email: emailVal,
      availabilityOption: availVal,
      users: usersVal
    };

    let updatedDevices;
    if (isEdit) {
      // Find the index of the original device
      const originalIndex = devices.indexOf(device);
      updatedDevices = devices.map((d, i) => i === originalIndex ? payloadDevice : d);
    } else {
      updatedDevices = [...devices, payloadDevice];
    }

    try {
      await request('POST', '/api/emails/ereader-devices', { ereaderDevices: updatedDevices });
      alert(isEdit ? 'Device updated successfully.' : 'Device added successfully.');
      closeModal();
      renderEmailsTab();
    } catch (err) {
      alert('Failed to save device: ' + err.message);
    }
  };
}

async function renderSharesTab() {
  const container = document.getElementById('tab-shares');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const shares = await request('GET', '/api/shares');

    container.innerHTML = `
      <div class="space-y-4 text-left">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold text-white">Active Share Links</h3>
        </div>

        <div class="border border-black-300 rounded-md bg-primary overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-black-400/60 text-black-100 text-xs uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">Shared Item ID</th>
                <th class="px-4 py-3">Slug / Link</th>
                <th class="px-4 py-3">Downloads / Limit</th>
                <th class="px-4 py-3">Embeddable</th>
                <th class="px-4 py-3">Password Protected</th>
                <th class="px-4 py-3">Expires At</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="shares-list-rows" class="divide-y divide-black-400">
              ${shares.length === 0 ? `
                <tr>
                  <td colspan="7" class="px-4 py-8 text-center text-black-100 text-xs">No active public share links found.</td>
                </tr>
              ` : shares.map(s => {
                const shareUrl = `${window.location.origin}/s/${s.id}`;
                const expiresStr = s.expiresAt ? (window.formatDateTime ? window.formatDateTime(s.expiresAt) : new Date(s.expiresAt).toLocaleString()) : 'Never';
                const limitStr = s.maxDownloads > 0 ? `${s.downloadsCount} / ${s.maxDownloads}` : `${s.downloadsCount} / Unlimited`;
                return `
                  <tr class="hover:bg-black-500/30 text-xs text-white">
                    <td class="px-4 py-3 font-mono text-black-100">${escapeHtml(s.libraryItemId)}</td>
                    <td class="px-4 py-3">
                      <a href="${shareUrl}" target="_blank" class="text-accent hover:underline flex items-center space-x-1">
                        <span>/s/${escapeHtml(s.id)}</span>
                        <span class="material-symbols text-xs">open_in_new</span>
                      </a>
                    </td>
                    <td class="px-4 py-3">${limitStr}</td>
                    <td class="px-4 py-3">${s.embeddable ? 'Yes' : 'No'}</td>
                    <td class="px-4 py-3">${s.hasPassword ? 'Yes' : 'No'}</td>
                    <td class="px-4 py-3 text-black-100">${expiresStr}</td>
                    <td class="px-4 py-3 text-right">
                      <button data-id="${s.id}" class="delete-share-btn bg-red-950 hover:bg-red-900 border border-red-500/50 hover:border-red-500 text-red-100 px-2.5 py-1 rounded text-xs transition-colors flex items-center space-x-1 ml-auto">
                        <span class="material-symbols text-xs">delete</span>
                        <span>Revoke</span>
                      </button>
                    </td>
                  </tr>
                `;
              }).join('')}
            </tbody>
          </table>
        </div>
      </div>
    `;

    // Bind delete events
    container.querySelectorAll('.delete-share-btn').forEach(btn => {
      btn.onclick = async () => {
        const id = btn.getAttribute('data-id');
        if (!confirm('Are you sure you want to revoke and delete this public share link? Any existing links will immediately stop working.')) {
          return;
        }
        try {
          await request('DELETE', `/api/share/mediaitem/${id}`);
          alert('Share link revoked successfully');
          renderSharesTab();
        } catch (err) {
          alert('Failed to revoke share link: ' + err.message);
        }
      };
    });

  } catch (err) {
    console.error(err);
    container.innerHTML = `<div class="text-red-500 text-xs text-center py-4">Failed to load public shares: ${err.message}</div>`;
  }
}

/**
 * Render the Active Login Sessions tab.
 * Displays all active refresh token/login sessions for the selected user, and allows revocation.
 */
async function renderLoginSessionsTab() {
  const container = document.getElementById('tab-login-sessions');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const users = await request('GET', '/api/users');
    const curUserId = window.currentUser ? window.currentUser.id : '';

    container.innerHTML = `
      <div class="space-y-4">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold text-white">Active Login Sessions</h3>
          <div class="flex items-center space-x-2">
            <label for="filter-login-session-user" class="text-xs text-black-100 uppercase tracking-wider">User:</label>
            <select id="filter-login-session-user" class="bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
              ${users.map(u => `<option value="${u.id}" ${u.id === curUserId ? 'selected' : ''}>${escapeHtml(u.username)}</option>`).join('')}
            </select>
          </div>
        </div>

        <div class="border border-black-300 rounded-md bg-primary overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-black-400/60 text-black-100 text-xs uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">Device / User Agent</th>
                <th class="px-4 py-3">IP Address</th>
                <th class="px-4 py-3">Created At</th>
                <th class="px-4 py-3">Last Active</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="login-sessions-list-rows" class="divide-y divide-black-400">
              <!-- Rows will be injected here -->
            </tbody>
          </table>
        </div>
      </div>
    `;

    const select = container.querySelector('#filter-login-session-user');
    select.onchange = () => {
      loadAndRenderLoginSessions(select.value);
    };

    // Load initial sessions for default selected user
    await loadAndRenderLoginSessions(select.value);

  } catch (err) {
    container.innerHTML = `<div class="text-red-500 text-center py-4">Failed to load active login sessions: ${escapeHtml(err.message)}</div>`;
  }
}

async function loadAndRenderLoginSessions(userId) {
  const tbody = document.getElementById('login-sessions-list-rows');
  if (!tbody) return;

  tbody.innerHTML = `
    <tr>
      <td colspan="5" class="px-4 py-8 text-center text-black-100">
        <div class="animate-spin rounded-full h-6 w-6 border-b-2 border-accent mx-auto"></div>
      </td>
    </tr>
  `;

  try {
    const sessions = await request('GET', `/api/users/${userId}/sessions`);
    tbody.innerHTML = '';

    if (sessions.length === 0) {
      tbody.innerHTML = `
        <tr>
          <td colspan="5" class="px-4 py-8 text-center text-black-100">
            No active login sessions found for this user.
          </td>
        </tr>
      `;
      return;
    }

    sessions.forEach(session => {
      const tr = document.createElement('tr');
      tr.className = 'hover:bg-black-500/30';

      const createdAtFormatted = session.createdAt ? (window.formatDateTime ? window.formatDateTime(session.createdAt) : new Date(session.createdAt).toLocaleString()) : 'Unknown';
      const updatedAtFormatted = session.updatedAt ? (window.formatDateTime ? window.formatDateTime(session.updatedAt) : new Date(session.updatedAt).toLocaleString()) : 'Unknown';

      const badgeHtml = session.isCurrent ? `
        <span class="ml-2 px-2 py-0.5 rounded text-xs bg-accent text-primary font-bold">Current</span>
      ` : '';

      const actionButtonHtml = `
        <button class="revoke-login-session-btn text-red-500 hover:text-red-400 font-semibold text-xs transition-colors duration-150" data-id="${session.id}">
          Revoke
        </button>
      `;

      tr.innerHTML = `
        <td class="px-4 py-3 text-white font-medium">
          <span class="font-mono text-xs break-all">${escapeHtml(session.userAgent || 'Unknown')}</span>
          ${badgeHtml}
        </td>
        <td class="px-4 py-3 text-black-100">${escapeHtml(session.ipAddress || 'Unknown')}</td>
        <td class="px-4 py-3 text-black-100 font-mono text-xs">${escapeHtml(createdAtFormatted)}</td>
        <td class="px-4 py-3 text-black-100 font-mono text-xs">${escapeHtml(updatedAtFormatted)}</td>
        <td class="px-4 py-3 text-right">${actionButtonHtml}</td>
      `;

      const revokeBtn = tr.querySelector('.revoke-login-session-btn');
      revokeBtn.onclick = async () => {
        const confirmMsg = session.isCurrent
          ? 'Are you sure you want to revoke your CURRENT login session? This will immediately log you out of this browser.'
          : 'Are you sure you want to revoke this login session?';

        if (confirm(confirmMsg)) {
          try {
            await request('DELETE', `/api/users/${userId}/sessions/${session.id}`);
            if (session.isCurrent) {
              await logout();
              window.location.reload();
            } else {
              loadAndRenderLoginSessions(userId);
            }
          } catch (err) {
            alert('Failed to revoke session: ' + err.message);
          }
        }
      };

      tbody.appendChild(tr);
    });

  } catch (err) {
    tbody.innerHTML = `
      <tr>
        <td colspan="5" class="px-4 py-8 text-center text-red-500">
          Failed to load sessions: ${escapeHtml(err.message)}
        </td>
      </tr>
    `;
  }
}

async function renderLibrariesTab() {
  const container = document.getElementById('tab-libraries');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const res = await request('GET', '/api/libraries?include=stats');
    const libs = res.libraries || [];

    let html = `
      <div class="space-y-6 bg-primary border border-black-300 p-6 rounded-md">
        <div class="flex justify-between items-center border-b border-black-400 pb-4">
          <div>
            <h3 class="text-lg font-semibold">Libraries</h3>
            <p class="text-xs text-black-100 mt-1">Configure and manage separate folders/categories for audiobooks and podcasts.</p>
          </div>
          <button type="button" id="btn-create-library" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-sm transition-opacity flex items-center space-x-1">
            <span class="material-symbols text-sm">add</span>
            <span>Add Library</span>
          </button>
        </div>

        <div class="space-y-4">
    `;

    if (libs.length === 0) {
      html += `
        <div class="text-center py-8 text-black-100">
          <p>No libraries configured. Click "Add Library" to get started.</p>
        </div>
      `;
    } else {
      libs.forEach(lib => {
        let mediaIcon = lib.icon === 'podcasts' || lib.mediaType === 'podcast' ? 'podcasts' : 'local_library';
        let foldersList = (lib.folders || []).map(f => f.path || f.fullPath).join(', ');
        if (!foldersList) foldersList = 'No folders configured';

        const isSelected = lib.id === getActiveLibraryId();
        const borderClass = isSelected ? 'border-l-warning' : 'border-l-black-400';

        html += `
          <div class="library-row border-y border-r ${borderClass} bg-black-500 rounded-r p-4 flex flex-col md:flex-row justify-between items-start md:items-center space-y-4 md:space-y-0 cursor-move transition-colors hover:bg-black-450" draggable="true" data-id="${lib.id}">
            <div class="flex items-center space-x-3 w-full md:w-auto">
              <!-- Reorder Handle -->
              <span class="material-symbols text-black-200 hover:text-white text-xl select-none mr-1">drag_handle</span>
              
              <!-- Content -->
              <div class="space-y-1">
                <div class="flex items-center space-x-2">
                  <span class="material-symbols text-lg text-accent">${mediaIcon}</span>
                  <span class="font-bold text-white">${escapeHtml(lib.name)}</span>
                  <span class="bg-black-400 text-black-50 text-[10px] px-1.5 py-0.5 rounded font-semibold uppercase">${lib.mediaType}</span>
                </div>
                <p class="text-xs text-black-100"><span class="font-semibold">Folders:</span> ${escapeHtml(foldersList)}</p>
                <p class="text-xs text-black-100"><span class="font-semibold">Provider:</span> ${escapeHtml(lib.provider || 'local')}</p>
              </div>
            </div>
            <div class="flex items-center space-x-2 w-full md:w-auto justify-end relative">
              <button type="button" class="btn-scan-lib bg-accent/10 hover:bg-accent/20 border border-accent/30 text-accent text-xs font-semibold px-3 py-1.5 rounded transition-colors" data-id="${lib.id}">Scan</button>
              
              <!-- Action Menu Dropdown Container -->
              <div class="relative inline-block text-left">
                <button type="button" class="btn-library-actions p-1.5 bg-black-400 hover:bg-black-350 text-white hover:text-accent rounded-full transition-colors focus:outline-none flex items-center justify-center" data-id="${lib.id}" title="Actions">
                  <span class="material-symbols text-lg">more_vert</span>
                </button>
                <div class="library-actions-menu absolute right-0 mt-1 w-32 bg-primary border border-black-300 rounded shadow-lg hidden py-1 z-50">
                  <button type="button" class="btn-edit-lib w-full text-left px-4 py-2 text-xs text-black-50 hover:bg-black-400 hover:text-white transition-colors flex items-center space-x-2" data-id="${lib.id}">
                    <span class="material-symbols text-sm">edit</span>
                    <span>Edit</span>
                  </button>
                  <button type="button" class="btn-delete-lib w-full text-left px-4 py-2 text-xs text-error hover:bg-black-400 hover:text-red-400 transition-colors flex items-center space-x-2" data-id="${lib.id}">
                    <span class="material-symbols text-sm">delete</span>
                    <span>Delete</span>
                  </button>
                </div>
              </div>
            </div>
          </div>
        `;
      });
    }

    html += `
        </div>
      </div>
    `;

    container.innerHTML = html;

    // Helper to update libraries order
    const updateLibrariesOrder = async () => {
      const rows = container.querySelectorAll('.library-row');
      const promises = [];
      rows.forEach((row, index) => {
        const id = row.dataset.id;
        promises.push(request('PATCH', `/api/libraries/${id}`, { displayOrder: index + 1 }));
      });
      try {
        await Promise.all(promises);
        const res = await request('GET', '/api/libraries');
        initLibrary(res);
      } catch (err) {
        console.error('Failed to update library display order:', err);
      }
    };

    // Attach HTML5 drag and drop events
    let draggedRow = null;
    const libraryRows = container.querySelectorAll('.library-row');
    libraryRows.forEach(row => {
      row.addEventListener('dragstart', (e) => {
        draggedRow = row;
        row.classList.add('opacity-40');
        e.dataTransfer.effectAllowed = 'move';
      });

      row.addEventListener('dragend', async () => {
        row.classList.remove('opacity-40');
        draggedRow = null;
        await updateLibrariesOrder();
      });

      row.addEventListener('dragover', (e) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
      });

      row.addEventListener('dragenter', (e) => {
        if (row !== draggedRow) {
          row.classList.add('bg-black-400');
        }
      });

      row.addEventListener('dragleave', () => {
        row.classList.remove('bg-black-400');
      });

      row.addEventListener('drop', (e) => {
        e.preventDefault();
        row.classList.remove('bg-black-400');
        if (row !== draggedRow) {
          const children = Array.from(row.parentNode.children);
          const draggedIndex = children.indexOf(draggedRow);
          const targetIndex = children.indexOf(row);
          if (draggedIndex < targetIndex) {
            row.parentNode.insertBefore(draggedRow, row.nextSibling);
          } else {
            row.parentNode.insertBefore(draggedRow, row);
          }
        }
      });
    });

    // Attach Event Listeners
    const createBtn = document.getElementById('btn-create-library');
    if (createBtn) {
      createBtn.onclick = () => showLibraryModal(null);
    }

    container.querySelectorAll('.btn-scan-lib').forEach(btn => {
      btn.onclick = async (e) => {
        e.stopPropagation();
        const id = btn.dataset.id;
        try {
          await request('POST', `/api/libraries/${id}/scan`);
          alert('Library scan requested successfully.');
        } catch (err) {
          alert('Failed to scan library: ' + err.message);
        }
      };
    });

    container.querySelectorAll('.btn-edit-lib').forEach(btn => {
      btn.onclick = (e) => {
        e.stopPropagation();
        const id = btn.dataset.id;
        const lib = libs.find(l => l.id === id);
        if (lib) showLibraryModal(lib);
      };
    });

    container.querySelectorAll('.btn-delete-lib').forEach(btn => {
      btn.onclick = async (e) => {
        e.stopPropagation();
        const id = btn.dataset.id;
        const lib = libs.find(l => l.id === id);
        if (!lib) return;
        if (confirm(`Are you sure you want to delete the library "${lib.name}"? This action cannot be undone.`)) {
          try {
            await request('DELETE', `/api/libraries/${id}`);
            alert('Library deleted successfully.');
            // Re-render tab and update dropdown
            await renderLibrariesTab();
            const updated = await request('GET', '/api/libraries');
            initLibrary(updated);
          } catch (err) {
            alert('Failed to delete library: ' + err.message);
          }
        }
      };
    });

    // Toggle Action Menus
    container.querySelectorAll('.btn-library-actions').forEach(btn => {
      btn.onclick = (e) => {
        e.stopPropagation();
        const menu = btn.nextElementSibling;
        const isOpen = !menu.classList.contains('hidden');
        
        // Close all other action menus
        container.querySelectorAll('.library-actions-menu').forEach(m => m.classList.add('hidden'));
        
        if (!isOpen) {
          menu.classList.remove('hidden');
        }
      };
    });

    // Close action menus when clicking outside
    if (!window.hasLibraryActionsListener) {
      window.hasLibraryActionsListener = true;
      document.addEventListener('click', () => {
        document.querySelectorAll('.library-actions-menu').forEach(m => m.classList.add('hidden'));
      });
    }

  } catch (err) {
    container.innerHTML = `
      <div class="bg-red-900/25 border border-red-900 text-red-200 p-4 rounded text-sm">
        Failed to load libraries: ${err.message}
      </div>
    `;
  }
}

function showLibraryModal(lib) {
  const isEdit = !!lib;
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 overflow-y-auto';

  // Extract settings
  const libSettings = lib?.settings || {};
  const coverAspectRatio = libSettings.coverAspectRatio || 1;

  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-lg p-6 rounded-md shadow-lg space-y-4 my-8">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2">${isEdit ? 'Edit Library' : 'Add Library'}</h3>
      
      <form id="library-form" class="space-y-4">
        <div>
          <label class="block text-xs text-black-100 mb-1">Library Name</label>
          <input type="text" id="lib-name" required value="${isEdit ? escapeHtml(lib.name) : ''}" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-xs text-black-100 mb-1">Media Type</label>
            <select id="lib-mediatype" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm" ${isEdit ? 'disabled' : ''}>
              <option value="book" ${isEdit && lib.mediaType === 'book' ? 'selected' : ''}>Book (Audiobooks / E-Books)</option>
              <option value="podcast" ${isEdit && lib.mediaType === 'podcast' ? 'selected' : ''}>Podcast</option>
            </select>
          </div>
          <div>
            <label class="block text-xs text-black-100 mb-1">Icon</label>
            <select id="lib-icon" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
              <option value="local_library" ${isEdit && lib.icon === 'local_library' ? 'selected' : ''}>Book/Library Icon</option>
              <option value="podcasts" ${isEdit && lib.icon === 'podcasts' ? 'selected' : ''}>Podcast Icon</option>
            </select>
          </div>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-xs text-black-100 mb-1">Provider</label>
            <select id="lib-provider" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
              <option value="local" ${isEdit && lib.provider === 'local' ? 'selected' : ''}>Local Metadata Only</option>
              <option value="audible" ${isEdit && lib.provider === 'audible' ? 'selected' : ''}>Audible</option>
              <option value="google" ${isEdit && lib.provider === 'google' ? 'selected' : ''}>Google Books</option>
              <option value="openlibrary" ${isEdit && lib.provider === 'openlibrary' ? 'selected' : ''}>Open Library</option>
              <option value="itunes" ${isEdit && lib.provider === 'itunes' ? 'selected' : ''}>iTunes</option>
            </select>
          </div>
          <div>
            <label class="block text-xs text-black-100 mb-1">Cover Aspect Ratio</label>
            <select id="lib-cover-aspect-ratio" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
              <option value="1" ${coverAspectRatio == 1 ? 'selected' : ''}>Square (1:1) - Podcasts/Audible</option>
              <option value="1.6" ${coverAspectRatio == 1.6 ? 'selected' : ''}>Standard Book (1.6:1)</option>
            </select>
          </div>
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1 flex justify-between items-center">
            <span>Library Folders</span>
            <button type="button" id="btn-add-folder-row" class="text-accent hover:underline text-xs font-semibold">Add Folder Path</button>
          </label>
          <div id="library-folders-container" class="space-y-2">
            <!-- Folder inputs go here -->
          </div>
          <p class="text-xs text-black-100 mt-1">Paths must be absolute, or relative to the server workspace (e.g. "/audiobooks" or "./audiobooks").</p>
        </div>

        <div class="flex justify-end space-x-2 pt-4 border-t border-black-400">
          <button type="button" id="close-lib-modal-btn" class="bg-black-400 hover:bg-black-350 px-4 py-2 rounded text-sm font-semibold text-white transition-colors">Cancel</button>
          <button type="submit" class="bg-accent hover:opacity-90 text-primary px-4 py-2 rounded text-sm font-semibold transition-opacity">Save</button>
        </div>
      </form>
    </div>
  `;

  document.body.appendChild(modal);

  const foldersContainer = modal.querySelector('#library-folders-container');
  const closeModal = () => modal.remove();
  modal.querySelector('#close-lib-modal-btn').onclick = closeModal;

  // Add folder row helper
  function addFolderRow(val = '', id = '') {
    const row = document.createElement('div');
    row.className = 'flex items-center space-x-2';
    row.innerHTML = `
      <input type="text" class="lib-folder-path flex-grow bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm" placeholder="e.g. /path/to/media" value="${escapeHtml(val)}" data-id="${id}">
      <button type="button" class="btn-remove-folder-row text-red-500 hover:text-red-400 material-symbols text-lg focus:outline-none">delete</button>
    `;
    row.querySelector('.btn-remove-folder-row').onclick = () => row.remove();
    foldersContainer.appendChild(row);
  }

  // Populate existing folders
  if (isEdit && lib.folders && lib.folders.length > 0) {
    lib.folders.forEach(f => {
      addFolderRow(f.path || f.fullPath, f.id);
    });
  } else {
    addFolderRow('');
  }

  // Hook add folder button
  modal.querySelector('#btn-add-folder-row').onclick = () => addFolderRow('');

  // Handle Form Submission
  const form = modal.querySelector('#library-form');
  form.onsubmit = async (e) => {
    e.preventDefault();

    const name = modal.querySelector('#lib-name').value;
    const mediaType = modal.querySelector('#lib-mediatype').value;
    const icon = modal.querySelector('#lib-icon').value;
    const provider = modal.querySelector('#lib-provider').value;
    const coverAspectRatioVal = parseFloat(modal.querySelector('#lib-cover-aspect-ratio').value);

    // Get folders
    const folderRows = modal.querySelectorAll('.lib-folder-path');
    const folders = [];
    let foldersValid = true;
    folderRows.forEach(row => {
      const pathVal = row.value.trim();
      const folderId = row.dataset.id || '';
      if (!pathVal) {
        foldersValid = false;
        return;
      }
      if (isEdit) {
        folders.push({
          id: folderId,
          path: pathVal,
          fullPath: pathVal
        });
      } else {
        folders.push({
          path: pathVal,
          fullPath: pathVal
        });
      }
    });

    if (!foldersValid || folders.length === 0) {
      alert('Please specify at least one valid folder path.');
      return;
    }

    const payload = {
      name,
      mediaType,
      icon,
      provider,
      settings: {
        coverAspectRatio: coverAspectRatioVal
      },
      folders
    };

    try {
      if (isEdit) {
        await request('PATCH', `/api/libraries/${lib.id}`, payload);
      } else {
        await request('POST', '/api/libraries', payload);
      }
      closeModal();
      // Re-render tab and rebuild library dropdown
      await renderLibrariesTab();
      const res = await request('GET', '/api/libraries');
      initLibrary(res);
    } catch (err) {
      alert('Failed to save library: ' + err.message);
    }
  };
}

export function applyServerThemeAndCss(settings) {
  if (!settings) return;
  const theme = settings.theme || 'dark';
  document.documentElement.setAttribute('data-theme', theme);

  let styleEl = document.getElementById('custom-css-style');
  if (settings.customCss) {
    if (!styleEl) {
      styleEl = document.createElement('style');
      styleEl.id = 'custom-css-style';
      document.head.appendChild(styleEl);
    }
    styleEl.textContent = settings.customCss;
  } else if (styleEl) {
    styleEl.remove();
  }
}



