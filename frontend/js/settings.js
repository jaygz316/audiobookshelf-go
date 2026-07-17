// frontend/js/settings.js (Proposed Implementation)
import { request, resolvePath } from './api.js';
import { getActiveLibraryId, getLibrariesList, initLibrary } from './library.js';
import { onEvent, offEvent, sendEvent } from './socket.js';
import { logout } from './auth.js';
import { showToast } from './toast.js';
import { renderUsersTab, renderApiKeysTab } from './settings/users.js';
import { renderBackupsTab } from './settings/backups.js';
import { renderLogsTab, renderListeningSessionsTab, renderLoginSessionsTab, renderTasksTab } from './settings/logs.js';

const activeScans = new Set();

export async function loadSettings() {
  const user = window.currentUser || {};
  if (user.type !== 'root' && user.type !== 'admin') {
    if (window.navigateTo) {
      window.navigateTo('/');
    } else {
      window.location.hash = '';
    }
    return;
  }

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
      <div role="tablist" aria-label="Settings categories" class="w-full md:w-64 flex-shrink-0 bg-primary/50 border border-black-400/50 rounded-lg p-2 flex flex-row md:flex-col overflow-x-auto md:overflow-x-visible space-x-1 md:space-x-0 space-y-0 md:space-y-1 h-fit scrollbar-none" id="settings-tabs">
        <!-- SERVER GROUP -->
        <div class="hidden md:block text-[10px] font-bold text-accent uppercase tracking-wider px-3 py-1 mt-2 mb-1">Server</div>
        <button role="tab" aria-selected="true" class="settings-tab-btn" data-tab="libraries">
          <span class="material-symbols text-lg">local_library</span>
          <span>Libraries</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="users">
          <span class="material-symbols text-lg">group</span>
          <span>Users</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="listening-sessions">
          <span class="material-symbols text-lg">insights</span>
          <span>Playback Sessions</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="backups">
          <span class="material-symbols text-lg">backup</span>
          <span>Backups</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="providers">
          <span class="material-symbols text-lg">api</span>
          <span>Custom Metadata Providers</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="logs">
          <span class="material-symbols text-lg">description</span>
          <span>System Logs</span>
        </button>

        <!-- CONFIGURATION GROUP -->
        <div class="hidden md:block text-[10px] font-bold text-accent uppercase tracking-wider px-3 py-1 mt-4 mb-1 border-t border-black-400/30 pt-3">Configuration</div>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="server">
          <span class="material-symbols text-lg">dns</span>
          <span>Server Settings</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="auth">
          <span class="material-symbols text-lg">security</span>
          <span>Authentication</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="notifications">
          <span class="material-symbols text-lg">notifications</span>
          <span>Notifications</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="emails">
          <span class="material-symbols text-lg">mail</span>
          <span>E-Reader Email</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="feeds">
          <span class="material-symbols text-lg">rss_feed</span>
          <span>RSS Feeds</span>
        </button>

        <!-- TOOLS GROUP -->
        <div class="hidden md:block text-[10px] font-bold text-accent uppercase tracking-wider px-3 py-1 mt-4 mb-1 border-t border-black-400/30 pt-3">Tools</div>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="apikeys">
          <span class="material-symbols text-lg">vpn_key</span>
          <span>API Keys</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="login-sessions">
          <span class="material-symbols text-lg">devices</span>
          <span>Login Sessions</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="upload">
          <span class="material-symbols text-lg">upload</span>
          <span>Uploads</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="shares">
          <span class="material-symbols text-lg">share</span>
          <span>Shares</span>
        </button>
        <button role="tab" aria-selected="false" class="settings-tab-btn" data-tab="tasks">
          <span class="material-symbols text-lg">downloading</span>
          <span>Tasks & Downloads</span>
        </button>
      </div>

      <!-- Right Content Column -->
      <div class="flex-grow bg-primary/20 border border-black-400/50 rounded-lg p-4 md:p-6 min-w-0" id="settings-tab-content">
        <div id="tab-libraries" class="space-y-6" role="tabpanel" aria-label="Libraries"></div>
        <div id="tab-users" class="space-y-6 hidden" role="tabpanel" aria-label="Users"></div>
        <div id="tab-apikeys" class="space-y-6 hidden" role="tabpanel" aria-label="API Keys"></div>
        <div id="tab-server" class="space-y-6 hidden" role="tabpanel" aria-label="Server Settings"></div>
        <div id="tab-auth" class="space-y-6 hidden" role="tabpanel" aria-label="Authentication"></div>
        <div id="tab-notifications" class="space-y-6 hidden" role="tabpanel" aria-label="Notifications"></div>
        <div id="tab-emails" class="space-y-6 hidden" role="tabpanel" aria-label="E-Reader Email"></div>
        <div id="tab-feeds" class="space-y-6 hidden" role="tabpanel" aria-label="RSS Feeds"></div>
        <div id="tab-listening-sessions" class="space-y-6 hidden" role="tabpanel" aria-label="Playback Sessions"></div>
        <div id="tab-login-sessions" class="space-y-6 hidden" role="tabpanel" aria-label="Login Sessions"></div>
        <div id="tab-backups" class="space-y-6 hidden" role="tabpanel" aria-label="Backups"></div>
        <div id="tab-upload" class="space-y-6 hidden" role="tabpanel" aria-label="Uploads"></div>
        <div id="tab-providers" class="space-y-6 hidden" role="tabpanel" aria-label="Custom Metadata Providers"></div>
        <div id="tab-shares" class="space-y-6 hidden" role="tabpanel" aria-label="Shares"></div>
        <div id="tab-logs" class="space-y-6 hidden" role="tabpanel" aria-label="System Logs"></div>
        <div id="tab-tasks" class="space-y-6 hidden" role="tabpanel" aria-label="Tasks & Downloads"></div>
      </div>
    </div>
  `;

  // Attach tab switcher click handlers
  const tabs = document.querySelectorAll('#settings-tabs button');
  tabs.forEach(tab => {
    tab.onclick = () => {
      const activeTabId = tab.dataset.tab;
      const updateTabs = () => {
        tabs.forEach(t => {
          t.setAttribute('aria-selected', 'false');
        });
        tab.setAttribute('aria-selected', 'true');

        document.querySelectorAll('#settings-tab-content > div').forEach(content => {
          if (content.id === `tab-${activeTabId}`) {
            content.classList.remove('hidden');
          } else {
            content.classList.add('hidden');
          }
        });
        if (activeTabId === 'tasks') {
          renderTasksTab();
        } else if (activeTabId === 'feeds') {
          renderFeedsTab();
        }
      };

      if (document.startViewTransition) {
        document.startViewTransition(() => {
          updateTabs();
        });
      } else {
        updateTabs();
      }
      
      // Update hash without triggering router reload
      window.location.hash = activeTabId;
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

  // Set initial tab from hash if present
  const hash = window.location.hash.substring(1);
  if (hash) {
    const targetTabBtn = Array.from(tabs).find(t => t.dataset.tab === hash);
    if (targetTabBtn) {
      targetTabBtn.click();
    }
  }
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
              <button type="button" id="btn-copy-opds" class="bg-accent hover:opacity-90 text-primary font-bold px-3 py-2 rounded transition-opacity flex items-center space-x-1 text-sm">
                <span class="material-symbols text-sm">content_copy</span>
                <span>Copy</span>
              </button>
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

        <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity flex items-center space-x-1.5 text-sm">
          <span class="material-symbols text-lg">save</span>
          <span>Save Server Settings</span>
        </button>
      </form>

      <form id="sorting-prefixes-form" class="space-y-6 bg-primary border border-black-300 p-6 rounded-md">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Sorting Prefixes (Title Ignore Prefixes)</h3>
        <p class="text-sm text-black-100">Titles starting with these words followed by a space will ignore them when sorting. For example, "The Hobbit" will sort as "Hobbit".</p>
        
        <div>
          <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Prefixes (Comma Separated)</label>
          <input type="text" id="setting-prefixes" value="${escapeHtml(prefixes.join(', '))}" placeholder="e.g. the, a, an, el, la" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
        </div>

        <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity flex items-center space-x-1.5 text-sm">
          <span class="material-symbols text-lg">save</span>
          <span>Save & Recompute Prefixes</span>
        </button>
      </form>

      <!-- Troubleshooting / Cache Tools -->
      <div class="space-y-6 bg-primary border border-black-300 p-6 rounded-md mt-6">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Troubleshooting / Cache Tools</h3>
        <p class="text-sm text-black-100">Perform maintenance operations on server caches and temporary storage.</p>
        
        <div class="flex flex-wrap gap-4 pt-2">
          <button type="button" id="btn-purge-all-cache" class="bg-black-400 hover:bg-black-300 text-white font-semibold px-4 py-2 rounded-md transition-colors border border-black-300 flex items-center space-x-1.5 text-sm">
            <span class="material-symbols text-lg">delete_sweep</span>
            <span>Purge All Cache</span>
          </button>
          <button type="button" id="btn-purge-items-cache" class="bg-black-400 hover:bg-black-300 text-white font-semibold px-4 py-2 rounded-md transition-colors border border-black-300 flex items-center space-x-1.5 text-sm">
            <span class="material-symbols text-lg">delete_sweep</span>
            <span>Purge Items Cache</span>
          </button>
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
            showToast('OPDS Feed URL copied to clipboard!', 'success');
          }).catch(err => {
            showToast('Failed to copy: ' + err, 'error');
          });
        }
      };
    }

    document.getElementById('server-settings-form').onsubmit = async (e) => {
      e.preventDefault();
      const btn = document.querySelector('#server-settings-form button[type="submit"]');
      if (btn) {
        btn.disabled = true;
        btn.innerHTML = `<span class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary inline-block mr-1.5"></span><span>Saving...</span>`;
      }
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
        showToast('Server settings saved successfully!', 'success');
      } catch (err) {
        showToast('Failed to save settings: ' + err.message, 'error');
      } finally {
        if (btn) {
          btn.disabled = false;
          btn.innerHTML = `<span class="material-symbols text-lg">save</span><span>Save Server Settings</span>`;
        }
      }
    };

    document.getElementById('sorting-prefixes-form').onsubmit = async (e) => {
      e.preventDefault();
      const btn = document.querySelector('#sorting-prefixes-form button[type="submit"]');
      if (btn) {
        btn.disabled = true;
        btn.innerHTML = `<span class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary inline-block mr-1.5"></span><span>Saving...</span>`;
      }
      try {
        const val = document.getElementById('setting-prefixes').value;
        const prefixArray = val.split(',').map(s => s.trim()).filter(Boolean);
        const res = await request('PATCH', '/api/sorting-prefixes', { sortingPrefixes: prefixArray });
        if (res && res.serverSettings) {
          window.serverSettings = res.serverSettings;
        }
        showToast('Sorting prefixes saved successfully!', 'success');
      } catch (err) {
        showToast('Failed to save prefixes: ' + err.message, 'error');
      } finally {
        if (btn) {
          btn.disabled = false;
          btn.innerHTML = `<span class="material-symbols text-lg">save</span><span>Save & Recompute Prefixes</span>`;
        }
      }
    };

    const btnPurgeAll = document.getElementById('btn-purge-all-cache');
    if (btnPurgeAll) {
      btnPurgeAll.onclick = async () => {
        const confirmed = await showConfirm(
          'Purge All Cache',
          'Are you sure you want to purge all cache? This includes resized cover images.',
          'Purge All',
          'Cancel'
        );
        if (!confirmed) return;
        try {
          btnPurgeAll.disabled = true;
          btnPurgeAll.innerHTML = `
            <span class="animate-spin rounded-full h-4 w-4 border-b-2 border-white inline-block mr-1.5"></span>
            <span>Purging...</span>
          `;
          await request('POST', '/api/cache/purge-all');
          showToast('Cache purged successfully!', 'success');
        } catch (err) {
          showToast('Failed to purge cache: ' + err.message, 'error');
        } finally {
          btnPurgeAll.disabled = false;
          btnPurgeAll.innerHTML = `
            <span class="material-symbols text-lg">delete_sweep</span>
            <span>Purge All Cache</span>
          `;
        }
      };
    }

    const btnPurgeItems = document.getElementById('btn-purge-items-cache');
    if (btnPurgeItems) {
      btnPurgeItems.onclick = async () => {
        const confirmed = await showConfirm(
          'Purge Cover Cache',
          'Are you sure you want to purge item cover cache? All resized cover images will be deleted.',
          'Purge Covers',
          'Cancel'
        );
        if (!confirmed) return;
        try {
          btnPurgeItems.disabled = true;
          btnPurgeItems.innerHTML = `
            <span class="animate-spin rounded-full h-4 w-4 border-b-2 border-white inline-block mr-1.5"></span>
            <span>Purging...</span>
          `;
          await request('POST', '/api/cache/purge-items');
          showToast('Items cover cache purged successfully!', 'success');
        } catch (err) {
          showToast('Failed to purge items cover cache: ' + err.message, 'error');
        } finally {
          btnPurgeItems.disabled = false;
          btnPurgeItems.innerHTML = `
            <span class="material-symbols text-lg">delete_sweep</span>
            <span>Purge Items Cache</span>
          `;
        }
      };
    }

  } catch (err) {
    container.innerHTML = `<p class="text-error text-sm">Failed to load server settings: ${err.message}</p>`;
  }
}

async function renderAuthSettingsTab() {
  const container = document.getElementById('tab-auth');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const auth = await request('GET', '/api/auth-settings');
    const activeMethods = auth.authActiveAuthMethods || ['local'];
    const hasManual = !!(auth.authOpenIDAuthorizationURL || auth.authOpenIDTokenURL || auth.authOpenIDUserInfoURL || auth.authOpenIDJwksURL || auth.authOpenIDLogoutURL);

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
              <label class="block text-xs text-black-100 mb-1">Match Existing Users By</label>
              <select id="oidc-match-existing-by" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
                <option value="" ${!auth.authOpenIDMatchExistingBy ? 'selected' : ''}>Do Not Match</option>
                <option value="email" ${auth.authOpenIDMatchExistingBy === 'email' ? 'selected' : ''}>Email</option>
                <option value="username" ${auth.authOpenIDMatchExistingBy === 'username' ? 'selected' : ''}>Username</option>
              </select>
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Group Claim</label>
              <input type="text" id="oidc-group-claim" value="${escapeHtml(auth.authOpenIDGroupClaim || '')}" placeholder="e.g. groups" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Advanced Permissions Claim</label>
              <input type="text" id="oidc-advanced-perms-claim" value="${escapeHtml(auth.authOpenIDAdvancedPermsClaim || '')}" placeholder="e.g. permissions" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Custom Login Message</label>
              <input type="text" id="oidc-custom-message" value="${escapeHtml(auth.authLoginCustomMessage || '')}" placeholder="Optional custom message on login screen" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div class="md:col-span-2">
              <label class="block text-xs text-black-100 mb-1">Mobile Redirect URIs</label>
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
            <label class="flex items-center space-x-3 cursor-pointer">
              <span class="abs-switch">
                <input type="checkbox" id="oidc-subfolder-redirects" ${auth.authOpenIDSubfolderForRedirectURLs ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Use subfolder for redirect URLs</span>
            </label>
          </div>

          <div class="pt-4 border-t border-black-400/60">
            <label class="flex items-center space-x-3 cursor-pointer select-none">
              <span class="abs-switch">
                <input type="checkbox" id="oidc-manual-endpoints-toggle" ${hasManual ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span class="text-sm font-semibold text-white">Configure Endpoints Manually</span>
            </label>
          </div>

          <div id="oidc-manual-endpoints-container" class="${hasManual ? '' : 'hidden'} transition-all grid grid-cols-1 md:grid-cols-2 gap-4 pt-4 border-t border-black-400/30">
            <div>
              <label class="block text-xs text-black-100 mb-1">Authorization URL</label>
              <input type="text" id="oidc-auth-url" value="${escapeHtml(auth.authOpenIDAuthorizationURL || '')}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Token URL</label>
              <input type="text" id="oidc-token-url" value="${escapeHtml(auth.authOpenIDTokenURL || '')}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">UserInfo URL</label>
              <input type="text" id="oidc-userinfo-url" value="${escapeHtml(auth.authOpenIDUserInfoURL || '')}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">JWKS URL</label>
              <input type="text" id="oidc-jwks-url" value="${escapeHtml(auth.authOpenIDJwksURL || '')}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Logout URL</label>
              <input type="text" id="oidc-logout-url" value="${escapeHtml(auth.authOpenIDLogoutURL || '')}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
            </div>
            <div>
              <label class="block text-xs text-black-100 mb-1">Token Signing Algorithm</label>
              <select id="oidc-signing-algorithm" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
                <option value="RS256" ${auth.authOpenIDTokenSigningAlgorithm === 'RS256' ? 'selected' : ''}>RS256 (Default)</option>
                <option value="RS384" ${auth.authOpenIDTokenSigningAlgorithm === 'RS384' ? 'selected' : ''}>RS384</option>
                <option value="RS512" ${auth.authOpenIDTokenSigningAlgorithm === 'RS512' ? 'selected' : ''}>RS512</option>
                <option value="ES256" ${auth.authOpenIDTokenSigningAlgorithm === 'ES256' ? 'selected' : ''}>ES256</option>
                <option value="ES384" ${auth.authOpenIDTokenSigningAlgorithm === 'ES384' ? 'selected' : ''}>ES384</option>
                <option value="ES512" ${auth.authOpenIDTokenSigningAlgorithm === 'ES512' ? 'selected' : ''}>ES512</option>
                <option value="HS256" ${auth.authOpenIDTokenSigningAlgorithm === 'HS256' ? 'selected' : ''}>HS256</option>
                <option value="HS384" ${auth.authOpenIDTokenSigningAlgorithm === 'HS384' ? 'selected' : ''}>HS384</option>
                <option value="HS512" ${auth.authOpenIDTokenSigningAlgorithm === 'HS512' ? 'selected' : ''}>HS512</option>
              </select>
            </div>
          </div>
        </div>

        <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity flex items-center space-x-1.5 text-sm">
          <span class="material-symbols text-lg">save</span>
          <span>Save Auth Settings</span>
        </button>
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

    const manualToggle = document.getElementById('oidc-manual-endpoints-toggle');
    const manualContainer = document.getElementById('oidc-manual-endpoints-container');
    if (manualToggle && manualContainer) {
      manualToggle.onchange = () => {
        if (manualToggle.checked) {
          manualContainer.classList.remove('hidden');
        } else {
          manualContainer.classList.add('hidden');
        }
      };
    }

    document.getElementById('auth-settings-form').onsubmit = async (e) => {
      e.preventDefault();
      const btn = document.querySelector('#auth-settings-form button[type="submit"]');
      if (btn) {
        btn.disabled = true;
        btn.innerHTML = `<span class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary inline-block mr-1.5"></span><span>Saving...</span>`;
      }
      try {
        const methods = [];
        if (document.getElementById('auth-method-local').checked) methods.push('local');
        if (document.getElementById('auth-method-openid').checked) methods.push('openid');

        if (methods.length === 0) {
          showToast('You must enable at least one authentication method.', 'warning');
          if (btn) {
            btn.disabled = false;
            btn.innerHTML = `<span class="material-symbols text-lg">save</span><span>Save Auth Settings</span>`;
          }
          return;
        }

        const isManual = document.getElementById('oidc-manual-endpoints-toggle').checked;
        const payload = {
          authActiveAuthMethods: methods,
          authOpenIDIssuerURL: document.getElementById('oidc-issuer').value,
          authOpenIDButtonText: document.getElementById('oidc-button-text').value,
          authOpenIDClientID: document.getElementById('oidc-client-id').value,
          authLoginCustomMessage: document.getElementById('oidc-custom-message').value,
          authOpenIDAutoLaunch: document.getElementById('oidc-autolaunch').checked,
          authOpenIDAutoRegister: document.getElementById('oidc-autoregister').checked,
          authOpenIDMatchExistingBy: document.getElementById('oidc-match-existing-by').value,
          authOpenIDGroupClaim: document.getElementById('oidc-group-claim').value,
          authOpenIDAdvancedPermsClaim: document.getElementById('oidc-advanced-perms-claim').value,
          authOpenIDSubfolderForRedirectURLs: document.getElementById('oidc-subfolder-redirects').checked,
          authOpenIDAuthorizationURL: isManual ? document.getElementById('oidc-auth-url').value : '',
          authOpenIDTokenURL: isManual ? document.getElementById('oidc-token-url').value : '',
          authOpenIDUserInfoURL: isManual ? document.getElementById('oidc-userinfo-url').value : '',
          authOpenIDJwksURL: isManual ? document.getElementById('oidc-jwks-url').value : '',
          authOpenIDLogoutURL: isManual ? document.getElementById('oidc-logout-url').value : '',
          authOpenIDTokenSigningAlgorithm: isManual ? document.getElementById('oidc-signing-algorithm').value : 'RS256'
        };

        const secretVal = document.getElementById('oidc-client-secret').value;
        if (secretVal && secretVal !== '••••••••') {
          payload.authOpenIDClientSecret = secretVal;
        }

        const mobileRedirects = document.getElementById('oidc-mobile-redirect').value;
        payload.authOpenIDMobileRedirectURIs = mobileRedirects.split(',').map(s => s.trim()).filter(Boolean);

        await request('PATCH', '/api/auth-settings', payload);
        showToast('Authentication settings saved successfully!', 'success');
      } catch (err) {
        showToast('Failed to save auth settings: ' + err.message, 'error');
      } finally {
        if (btn) {
          btn.disabled = false;
          btn.innerHTML = `<span class="material-symbols text-lg">save</span><span>Save Auth Settings</span>`;
        }
      }
    };

  } catch (err) {
    container.innerHTML = `<p class="text-error text-sm">Failed to load auth settings: ${err.message}</p>`;
  }
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
            <div class="flex rounded bg-black-600 p-1 border border-black-300 w-full" id="prov-mediatype-toggle-container">
              <button type="button" id="prov-mediatype-book-btn" class="flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none bg-success text-white" data-value="book">Book</button>
              <button type="button" id="prov-mediatype-podcast-btn" class="flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white" data-value="podcast">Podcast</button>
            </div>
            <input type="hidden" id="prov-mediatype" value="book">
          </div>
          <div>
            <label class="block text-xs text-black-100 mb-1">Authorization Header Value (Optional)</label>
            <input type="text" id="prov-auth" placeholder="Bearer my-secret-token" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
          </div>

          <button type="submit" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-2 rounded transition-opacity text-sm flex items-center justify-center space-x-1.5">
            <span class="material-symbols text-lg">save</span>
            <span>Add Provider</span>
          </button>
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

    const provMediatypeInput = container.querySelector('#prov-mediatype');
    const provBookBtn = container.querySelector('#prov-mediatype-book-btn');
    const provPodcastBtn = container.querySelector('#prov-mediatype-podcast-btn');

    if (provBookBtn && provPodcastBtn && provMediatypeInput) {
      provBookBtn.onclick = () => {
        provMediatypeInput.value = 'book';
        provBookBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none bg-success text-white';
        provPodcastBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white';
      };

      provPodcastBtn.onclick = () => {
        provMediatypeInput.value = 'podcast';
        provBookBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white';
        provPodcastBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none bg-success text-white';
      };
    }

    document.getElementById('create-provider-form').onsubmit = async (e) => {
      e.preventDefault();
      const btn = document.querySelector('#create-provider-form button[type="submit"]');
      if (btn) {
        btn.disabled = true;
        btn.innerHTML = `<span class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary inline-block mr-1.5"></span><span>Adding...</span>`;
      }
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
        showToast('Custom metadata provider created!', 'success');
        renderProvidersTab(); // reload
      } catch (err) {
        showToast('Failed to add provider: ' + err.message, 'error');
      } finally {
        if (btn) {
          btn.disabled = false;
          btn.innerHTML = `<span class="material-symbols text-lg">save</span><span>Add Provider</span>`;
        }
      }
    };

  } catch (err) {
    container.innerHTML = `<p class="text-error text-sm">Failed to load metadata providers: ${err.message}</p>`;
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
        <button class="delete-prov-btn bg-red-900/40 hover:bg-red-900/60 border border-red-500/30 text-error hover:text-white hover:border-red-500/50 text-xs font-semibold px-2.5 py-1 rounded inline-flex items-center space-x-1 transition-colors cursor-pointer" data-id="${p.id}">
          <span class="material-symbols text-sm">delete</span>
          <span>Delete</span>
        </button>
      </td>
    `;

    tr.querySelector('.delete-prov-btn').onclick = async () => {
      const confirmed = await showConfirm(
        'Delete Custom Provider',
        `Are you sure you want to delete custom provider "${p.name}"? Any libraries using it will fallback to defaults.`,
        'Delete',
        'Cancel'
      );
      if (!confirmed) return;
      try {
        await request('DELETE', `/api/custom-metadata-providers/${p.id}`);
        renderProvidersTab(); // reload
        showToast('Custom provider deleted successfully.', 'success');
      } catch (err) {
        showToast('Delete failed: ' + err.message, 'error');
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
      showToast('Please select at least one file to upload.', 'warning');
      return;
    }

    const libraryId = getActiveLibraryId();
    if (!libraryId) {
      showToast('No active library selected. Please select a library first.', 'warning');
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
      showToast(`${files.length} file(s) uploaded successfully! The library will be scanned for new items.`, 'success');
    } catch (err) {
      progressLabel.textContent = 'Upload failed: ' + err.message;
      showToast('Upload failed: ' + err.message, 'error');
    } finally {
      btn.disabled = false;
    }
  };
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

        <button type="submit" id="save-apprise-settings-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity flex items-center space-x-1.5 text-sm">
          <span class="material-symbols text-lg">save</span>
          <span>Save General Settings</span>
        </button>
      </form>

      <hr class="border-black-400">

      <div class="space-y-4">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold text-white">Notification Setups</h3>
          <button id="add-notification-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity flex items-center space-x-1">
            <span class="material-symbols text-sm">add</span>
            <span>Create</span>
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
      const btn = document.getElementById('save-apprise-settings-btn');
      if (btn) {
        btn.disabled = true;
        btn.innerHTML = `<span class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary inline-block mr-1.5"></span><span>Saving...</span>`;
      }
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
        showToast('Notification settings saved successfully!', 'success');
        renderNotificationsTab();
      } catch (err) {
        showToast('Failed to save settings: ' + err.message, 'error');
      } finally {
        if (btn) {
          btn.disabled = false;
          btn.innerHTML = `<span class="material-symbols text-lg">save</span><span>Save General Settings</span>`;
        }
      }
    };

    document.getElementById('add-notification-btn').onclick = () => {
      triggerCreateNotificationModal(settings, () => {
        renderNotificationsTab();
      });
    };

  } catch (err) {
    container.innerHTML = `<p class="text-error text-sm">Failed to load notifications settings: ${escapeHtml(err.message)}</p>`;
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
      ? `<span class="bg-green-900/50 text-success text-[10px] px-1.5 py-0.5 rounded font-normal uppercase">Enabled</span>`
      : `<span class="bg-red-900/50 text-error text-[10px] px-1.5 py-0.5 rounded font-normal uppercase">Disabled</span>`;

    tr.innerHTML = `
      <td class="px-4 py-3 font-mono text-xs text-white">${escapeHtml(notif.id || '')}</td>
      <td class="px-4 py-3 text-black-50">${escapeHtml(notif.eventName || '')}</td>
      <td class="px-4 py-3">${enabledBadge}</td>
      <td class="px-4 py-3 text-right">
        <button class="delete-notif-btn text-error hover:text-red-400 font-semibold text-xs inline-flex items-center space-x-1" data-id="${notif.id}">
          <span class="material-symbols text-sm">delete</span>
          <span>Delete</span>
        </button>
      </td>
    `;

    const deleteBtn = tr.querySelector('.delete-notif-btn');
    deleteBtn.onclick = async () => {
      const confirmed = await showConfirm(
        'Delete Notification Setup',
        'Are you sure you want to delete this notification setup?',
        'Delete',
        'Cancel'
      );
      if (confirmed) {
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
          showToast('Notification setup deleted successfully.', 'success');
          renderNotificationsTab();
        } catch (err) {
          showToast('Failed to delete notification setup: ' + err.message, 'error');
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
          <button type="button" id="close-notif-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold transition-colors flex items-center space-x-1">
            <span class="material-symbols text-sm">close</span>
            <span>Cancel</span>
          </button>
          <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity flex items-center space-x-1">
            <span class="material-symbols text-sm">check</span>
            <span>Create</span>
          </button>
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
    const btn = form.querySelector('button[type="submit"]');
    if (btn) {
      btn.disabled = true;
      btn.innerHTML = `<span class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary inline-block mr-1"></span><span>Creating...</span>`;
    }

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
      showToast(`Failed to save notification setup: ${err.message}`, 'error');
      if (btn) {
        btn.disabled = false;
        btn.innerHTML = `<span class="material-symbols text-sm">check</span><span>Create</span>`;
      }
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

    const typeIcons = {
      book: 'book',
      podcast: 'podcasts',
      playlist: 'playlist_play',
      collection: 'bookmarks',
      series: 'layers'
    };

    const typeLabels = {
      book: 'Book',
      podcast: 'Podcast',
      playlist: 'Playlist',
      collection: 'Collection',
      series: 'Series'
    };

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
            showToast('Please create a Podcast library first to import/export OPML.', 'warning');
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
      const icon = typeIcons[feed.type] || 'rss_feed';
      const label = typeLabels[feed.type] || (feed.type ? feed.type.charAt(0).toUpperCase() + feed.type.slice(1) : 'Unknown');

      tr.innerHTML = `
        <td class="px-4 py-3 font-medium text-white flex items-center gap-2">
          <span class="material-symbols text-lg text-black-200/80">${icon}</span>
          <span>${escapeHtml(feed.title || feed.entityId)}</span>
        </td>
        <td class="px-4 py-3 text-black-50 text-xs">${escapeHtml(label)}</td>
        <td class="px-4 py-3 text-black-100">
          <div class="flex items-center gap-3">
            <span class="truncate max-w-xs font-mono text-xs select-all text-black-100">${escapeHtml(feed.feedUrl)}</span>
            <button class="copy-feed-btn text-accent hover:text-accent-hover font-semibold text-xs flex items-center gap-1" data-url="${escapeHtml(feed.feedUrl)}">
              <span class="material-symbols text-sm pointer-events-none">content_copy</span>
              Copy
            </button>
          </div>
        </td>
        <td class="px-4 py-3 text-right">
          <button class="delete-feed-btn text-error hover:text-red-400 font-semibold text-xs inline-flex items-center gap-1" data-id="${escapeHtml(feed.id)}">
            <span class="material-symbols text-sm pointer-events-none">close</span>
            Close Feed
          </button>
        </td>
      `;

      // Wire up copy button
      tr.querySelector('.copy-feed-btn').onclick = (e) => {
        const btn = e.currentTarget;
        navigator.clipboard.writeText(btn.dataset.url).then(() => {
          showToast('Feed URL copied to clipboard', 'success');
        }).catch(err => {
          showToast('Failed to copy feed URL: ' + err.message, 'error');
        });
      };

      // Wire up delete button
      tr.querySelector('.delete-feed-btn').onclick = async (e) => {
        const confirmed = await showConfirm(
          'Close RSS Feed',
          'Are you sure you want to close this RSS feed?',
          'Close Feed',
          'Cancel'
        );
        if (!confirmed) return;
        const btn = e.currentTarget;
        try {
          await request('DELETE', `/api/feeds/${btn.dataset.id}`);
          showToast('RSS feed closed successfully', 'success');
          renderFeedsTab();
        } catch (err) {
          showToast('Failed to close feed: ' + err.message, 'error');
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
    const usersResp = await request('GET', '/api/users');
    const users = usersResp.users || [];

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
          <button type="submit" id="save-email-settings-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity text-sm flex items-center space-x-1.5">
            <span class="material-symbols text-lg">save</span>
            <span>Save Settings</span>
          </button>
          <button type="button" id="test-email-settings-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-white font-bold px-4 py-2 rounded transition-colors text-sm flex items-center space-x-1.5">
            <span class="material-symbols text-lg">mail</span>
            <span>Send Test Email</span>
          </button>
        </div>
      </form>

      <hr class="border-black-400">

      <div class="space-y-4">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold text-white">E-Reader Devices</h3>
          <button id="add-ereader-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity flex items-center space-x-1.5">
            <span class="material-symbols text-sm">devices</span>
            <span>Add Device</span>
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
      const btn = document.getElementById('save-email-settings-btn');
      if (btn) {
        btn.disabled = true;
        btn.innerHTML = `<span class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary inline-block mr-1.5"></span><span>Saving...</span>`;
      }
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
        showToast('SMTP configuration saved successfully!', 'success');
        renderEmailsTab();
      } catch (err) {
        showToast('Failed to save configuration: ' + err.message, 'error');
        if (btn) {
          btn.disabled = false;
          btn.innerHTML = `<span class="material-symbols text-lg">save</span><span>Save Settings</span>`;
        }
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
          showToast('Please specify a Test Recipient Address', 'warning');
          return;
        }

        const btn = document.getElementById('test-email-settings-btn');
        btn.innerHTML = `
          <span class="animate-spin rounded-full h-4 w-4 border-b-2 border-white inline-block mr-2"></span>
          <span>Sending...</span>
        `;
        btn.disabled = true;

        try {
          await request('POST', '/api/emails/test', payload);
          showToast('Test email sent successfully! Please check the recipient address.', 'success');
        } finally {
          btn.innerHTML = `
            <span class="material-symbols text-lg">mail</span>
            <span>Send Test Email</span>
          `;
          btn.disabled = false;
        }
      } catch (err) {
        showToast('Failed to send test email: ' + err.message, 'error');
      }
    };

    // Add Ereader Device Handler
    document.getElementById('add-ereader-btn').onclick = () => {
      triggerEreaderDeviceModal(null, devices, users, settings);
    };

  } catch (err) {
    container.innerHTML = `<p class="text-error text-sm">Failed to load email settings: ${escapeHtml(err.message)}</p>`;
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
      availText = '<span class="text-warning">Admin or Up</span>';
    } else if (dev.availabilityOption === 'specificUsers') {
      const allowedNames = (dev.users || []).map(uId => {
        const u = users.find(x => x.id === uId);
        return u ? u.username : uId;
      }).join(', ');
      availText = `<span class="text-info font-semibold">Specific Users</span> <span class="text-xs text-black-100">(${escapeHtml(allowedNames || 'none')})</span>`;
    }

    tr.innerHTML = `
      <td class="px-4 py-3 font-semibold text-white">${escapeHtml(dev.name)}</td>
      <td class="px-4 py-3 font-mono text-black-50">${escapeHtml(dev.email)}</td>
      <td class="px-4 py-3 text-sm">${availText}</td>
      <td class="px-4 py-3 text-right space-x-2">
        <button class="edit-dev-btn text-accent hover:text-accent-hover text-xs font-semibold inline-flex items-center space-x-1" data-index="${idx}">
          <span class="material-symbols text-sm">edit</span>
          <span>Edit</span>
        </button>
        <button class="delete-dev-btn text-error hover:text-red-400 text-xs font-semibold inline-flex items-center space-x-1" data-index="${idx}">
          <span class="material-symbols text-sm">delete</span>
          <span>Delete</span>
        </button>
      </td>
    `;

    tr.querySelector('.edit-dev-btn').onclick = (e) => {
      const btn = e.currentTarget;
      const index = parseInt(btn.dataset.index, 10);
      triggerEreaderDeviceModal(devices[index], devices, users, settings);
    };

    tr.querySelector('.delete-dev-btn').onclick = async (e) => {
      const btn = e.currentTarget;
      const index = parseInt(btn.dataset.index, 10);
      const confirmed = await showConfirm(
        'Delete Device',
        `Are you sure you want to delete device "${devices[index].name}"?`,
        'Delete',
        'Cancel'
      );
      if (!confirmed) return;

      const updatedDevices = devices.filter((_, i) => i !== index);
      try {
        await request('POST', '/api/emails/ereader-devices', { ereaderDevices: updatedDevices });
        showToast('Device deleted successfully.', 'success');
        renderEmailsTab();
      } catch (err) {
        showToast('Failed to delete device: ' + err.message, 'error');
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
          <button type="button" id="close-ereader-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold transition-colors flex items-center space-x-1">
            <span class="material-symbols text-sm">close</span>
            <span>Cancel</span>
          </button>
          <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity flex items-center space-x-1">
            <span class="material-symbols text-sm">check</span>
            <span>Save</span>
          </button>
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
    const btn = form.querySelector('button[type="submit"]');
    if (btn) {
      btn.disabled = true;
      btn.innerHTML = `<span class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary inline-block mr-1"></span><span>Saving...</span>`;
    }

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
      showToast(isEdit ? 'Device updated successfully.' : 'Device added successfully.', 'success');
      closeModal();
      renderEmailsTab();
    } catch (err) {
      showToast('Failed to save device: ' + err.message, 'error');
      if (btn) {
        btn.disabled = false;
        btn.innerHTML = `<span class="material-symbols text-sm">check</span><span>Save</span>`;
      }
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
                      <button data-id="${s.id}" class="delete-share-btn bg-red-900/40 hover:bg-red-900/60 border border-red-500/30 text-error hover:text-white hover:border-red-500/50 px-2.5 py-1 rounded text-xs transition-colors flex items-center space-x-1 ml-auto cursor-pointer">
                        <span class="material-symbols text-xs">link_off</span>
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
        const confirmed = await showConfirm(
          'Revoke Share Link',
          'Are you sure you want to revoke and delete this public share link? Any existing links will immediately stop working.',
          'Revoke Link',
          'Cancel'
        );
        if (!confirmed) {
          return;
        }
        try {
          await request('DELETE', `/api/share/mediaitem/${id}`);
          showToast('Share link revoked successfully', 'success');
          renderSharesTab();
        } catch (err) {
          showToast('Failed to revoke share link: ' + err.message, 'error');
        }
      };
    });

  } catch (err) {
    console.error(err);
    container.innerHTML = `<div class="text-error text-xs text-center py-4">Failed to load public shares: ${err.message}</div>`;
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
        <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 sm:gap-0 border-b border-black-400 pb-4">
          <div>
            <h3 class="text-lg font-semibold">Libraries</h3>
            <p class="text-xs text-black-100 mt-1">Configure and manage separate folders/categories for audiobooks and podcasts.</p>
          </div>
          <button type="button" id="btn-create-library" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-sm transition-opacity flex items-center space-x-1 flex-shrink-0 w-full sm:w-auto justify-center sm:justify-start">
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
        const borderClass = isSelected ? 'border-accent' : 'border-black-300 hover:border-accent/50';
        const isScanning = activeScans.has(lib.id);
        const spinClass = isScanning ? 'animate-spin' : '';

        html += `
          <div class="library-row border ${borderClass} border-l-4 ${isSelected ? 'border-l-accent' : 'border-l-transparent'} bg-black-500 rounded p-4 flex flex-col md:flex-row justify-between items-start md:items-center space-y-4 md:space-y-0 transition-colors hover:bg-black-400" draggable="true" data-id="${lib.id}">
            <div class="flex items-center space-x-3 w-full md:w-auto">
              <!-- Reorder Handle -->
              <span class="drag-handle material-symbols text-black-200 hover:text-white text-xl select-none mr-1 cursor-grab active:cursor-grabbing" title="Drag to reorder">drag_handle</span>
              
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
              <button type="button" class="btn-scan-lib bg-accent/10 hover:bg-accent/20 border border-accent/30 text-accent text-xs font-semibold px-3 py-1.5 rounded transition-colors flex items-center space-x-1" data-id="${lib.id}">
                <span class="material-symbols text-sm ${spinClass}">sync</span>
                <span>Scan</span>
              </button>
              
              <!-- Action Menu Dropdown Container -->
              <div class="relative inline-block text-left">
                <button type="button" class="btn-library-actions p-1.5 bg-black-400 hover:bg-black-300 text-white hover:text-accent rounded-full transition-colors focus:outline-none flex items-center justify-center" data-id="${lib.id}" title="Actions">
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
        // Only allow dragging from the drag handle
        const handle = e.target.closest('.drag-handle');
        if (!handle) {
          e.preventDefault();
          return;
        }
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
          row.classList.add('bg-black-400', 'border-accent');
          row.classList.remove('border-black-300');
        }
      });

      const resetRowStyles = () => {
        row.classList.remove('bg-black-400');
        if (row.dataset.id === getActiveLibraryId()) {
          row.classList.add('border-accent');
          row.classList.remove('border-black-300', 'hover:border-accent/50');
        } else {
          row.classList.remove('border-accent');
          row.classList.add('border-black-300');
        }
      };

      row.addEventListener('dragleave', resetRowStyles);

      row.addEventListener('drop', (e) => {
        e.preventDefault();
        resetRowStyles();
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
          showToast('Library scan requested successfully.', 'success');
        } catch (err) {
          showToast('Failed to scan library: ' + err.message, 'error');
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
        const confirmed = await showConfirm(
          'Delete Library',
          `Are you sure you want to delete the library "${lib.name}"? This action cannot be undone.`,
          'Delete',
          'Cancel'
        );
        if (confirmed) {
          try {
            await request('DELETE', `/api/libraries/${id}`);
            showToast('Library deleted successfully.', 'success');
            // Re-render tab and update dropdown
            await renderLibrariesTab();
            const updated = await request('GET', '/api/libraries');
            initLibrary(updated);
          } catch (err) {
            showToast('Failed to delete library: ' + err.message, 'error');
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
      <div class="bg-red-900/40 border border-red-500/30 text-error p-4 rounded text-sm">
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
    <div class="bg-primary border border-black-300 w-full max-w-lg p-6 rounded-md shadow-lg space-y-4 my-8 relative">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2">${isEdit ? 'Edit Library' : 'Add Library'}</h3>
      
      <form id="library-form" class="space-y-4">
        <div>
          <label class="block text-xs text-black-100 mb-1">Library Name</label>
          <input type="text" id="lib-name" required value="${isEdit ? escapeHtml(lib.name) : ''}" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-xs text-black-100 mb-1">Media Type</label>
            <div class="flex rounded bg-black-600 p-1 border border-black-300 w-full" id="lib-mediatype-toggle-container">
              <button type="button" id="lib-mediatype-book-btn" class="flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none ${(!isEdit || lib.mediaType === 'book') ? 'bg-success text-white' : 'text-black-100 hover:text-white'}" ${isEdit ? 'disabled' : ''} data-value="book">Book</button>
              <button type="button" id="lib-mediatype-podcast-btn" class="flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none ${(isEdit && lib.mediaType === 'podcast') ? 'bg-success text-white' : 'text-black-100 hover:text-white'}" ${isEdit ? 'disabled' : ''} data-value="podcast">Podcast</button>
            </div>
            <input type="hidden" id="lib-mediatype" value="${isEdit ? lib.mediaType : 'book'}">
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
            <div class="flex rounded bg-black-600 p-1 border border-black-300 w-full" id="lib-aspect-toggle-container">
              <button type="button" id="lib-aspect-square-btn" class="flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none ${(coverAspectRatio == 1) ? 'bg-success text-white' : 'text-black-100 hover:text-white'}" data-value="1">Square (1:1)</button>
              <button type="button" id="lib-aspect-standard-btn" class="flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none ${(coverAspectRatio == 1.6) ? 'bg-success text-white' : 'text-black-100 hover:text-white'}" data-value="1.6">Standard Book (1.6:1)</button>
            </div>
            <input type="hidden" id="lib-cover-aspect-ratio" value="${coverAspectRatio}">
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
          <button type="button" id="close-lib-modal-btn" class="bg-black-400 hover:bg-black-300 px-4 py-2 rounded text-sm font-semibold text-white transition-colors flex items-center space-x-1">
            <span class="material-symbols text-sm">close</span>
            <span>Cancel</span>
          </button>
          <button type="submit" class="bg-accent hover:opacity-90 text-primary px-4 py-2 rounded text-sm font-semibold transition-opacity flex items-center space-x-1">
            <span class="material-symbols text-sm">check</span>
            <span>Save</span>
          </button>
        </div>
      </form>
    </div>
  `;

  document.body.appendChild(modal);

  // Handle Media Type Toggle click events (if not editing)
  if (!isEdit) {
    const mediatypeInput = modal.querySelector('#lib-mediatype');
    const bookBtn = modal.querySelector('#lib-mediatype-book-btn');
    const podcastBtn = modal.querySelector('#lib-mediatype-podcast-btn');

    bookBtn.onclick = () => {
      mediatypeInput.value = 'book';
      bookBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none bg-success text-white';
      podcastBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white';
      
      // Auto switch icon and cover aspect ratio defaults when switching media type
      const iconSelect = modal.querySelector('#lib-icon');
      if (iconSelect) iconSelect.value = 'local_library';

      const aspectInput = modal.querySelector('#lib-cover-aspect-ratio');
      const aspectSquareBtn = modal.querySelector('#lib-aspect-square-btn');
      const aspectStandardBtn = modal.querySelector('#lib-aspect-standard-btn');
      if (aspectInput && aspectSquareBtn && aspectStandardBtn) {
        aspectInput.value = '1.6';
        aspectSquareBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white';
        aspectStandardBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none bg-success text-white';
      }
    };

    podcastBtn.onclick = () => {
      mediatypeInput.value = 'podcast';
      bookBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white';
      podcastBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none bg-success text-white';
      
      // Auto switch icon and cover aspect ratio defaults when switching media type
      const iconSelect = modal.querySelector('#lib-icon');
      if (iconSelect) iconSelect.value = 'podcasts';

      const aspectInput = modal.querySelector('#lib-cover-aspect-ratio');
      const aspectSquareBtn = modal.querySelector('#lib-aspect-square-btn');
      const aspectStandardBtn = modal.querySelector('#lib-aspect-standard-btn');
      if (aspectInput && aspectSquareBtn && aspectStandardBtn) {
        aspectInput.value = '1';
        aspectSquareBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none bg-success text-white';
        aspectStandardBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white';
      }
    };
  }

  // Handle Cover Aspect Ratio Toggle click events
  const aspectInput = modal.querySelector('#lib-cover-aspect-ratio');
  const aspectSquareBtn = modal.querySelector('#lib-aspect-square-btn');
  const aspectStandardBtn = modal.querySelector('#lib-aspect-standard-btn');

  aspectSquareBtn.onclick = () => {
    aspectInput.value = '1';
    aspectSquareBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none bg-success text-white';
    aspectStandardBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white';
  };

  aspectStandardBtn.onclick = () => {
    aspectInput.value = '1.6';
    aspectSquareBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white';
    aspectStandardBtn.className = 'flex-grow py-1.5 text-xs font-semibold rounded text-center transition-all focus:outline-none bg-success text-white';
  };

  const foldersContainer = modal.querySelector('#library-folders-container');
  const closeModal = () => modal.remove();
  modal.querySelector('#close-lib-modal-btn').onclick = closeModal;

  // Open Folder Picker Helper
  function openFolderPicker(inputEl) {
    const pickerContainer = document.createElement('div');
    pickerContainer.className = 'absolute inset-0 bg-primary rounded-md p-6 flex flex-col z-20 space-y-4';
    
    pickerContainer.innerHTML = `
      <div class="flex items-center justify-between border-b border-black-400 pb-2">
        <h4 class="text-sm font-bold text-white flex items-center">
          <span class="material-symbols text-lg mr-1.5">folder_open</span>
          <span>Select Folder</span>
        </h4>
        <button type="button" id="btn-picker-close" class="text-gray-400 hover:text-white material-symbols text-lg focus:outline-none">close</button>
      </div>

      <!-- Current path display and navigation up -->
      <div class="flex items-center space-x-2 bg-black-500/70 p-2 rounded border border-black-300/30 text-xs">
        <button type="button" id="btn-picker-up" class="text-accent hover:opacity-85 material-symbols text-base focus:outline-none flex items-center justify-center" title="Go up one folder">arrow_upward</button>
        <span class="text-black-100 select-none">Path:</span>
        <span id="picker-current-path" class="font-mono text-white grow flex items-center overflow-x-auto whitespace-nowrap scrollbar-none select-none">/</span>
      </div>

      <!-- Main columns flex layout -->
      <div class="flex flex-grow min-h-0 border border-black-300 bg-black-600 rounded overflow-hidden text-sm h-[260px]">
        <!-- Directories list (left column) -->
        <div class="w-1/2 border-r border-black-300 flex flex-col h-full">
          <div class="bg-black-500 px-3 py-1.5 text-xs text-black-50 font-semibold border-b border-black-300">Folders</div>
          <div id="picker-dirs-list" class="flex-grow overflow-y-auto p-1 divide-y divide-black-400/20">
            <!-- Dynamically populated -->
          </div>
        </div>
        <!-- Subdirectories list (right column) -->
        <div class="w-1/2 flex flex-col h-full">
          <div class="bg-black-500 px-3 py-1.5 text-xs text-black-50 font-semibold border-b border-black-300">Subfolders</div>
          <div id="picker-subdirs-list" class="flex-grow overflow-y-auto p-1 divide-y divide-black-400/20">
            <!-- Dynamically populated -->
          </div>
        </div>
      </div>

      <!-- Footer controls -->
      <div class="flex items-center justify-between pt-2 border-t border-black-400">
        <div class="text-xxs text-black-100 flex items-center mr-2">
          <span class="material-symbols text-xs mr-1 text-accent flex items-center justify-center">info</span>
          <span class="leading-none">Click to inspect subfolders. Double-click to open.</span>
        </div>
        <div class="flex space-x-2">
          <button type="button" id="btn-picker-cancel" class="bg-black-400 hover:bg-black-300 px-3 py-1.5 rounded text-xs font-semibold text-white transition-colors flex items-center space-x-1">
            <span class="material-symbols text-sm">close</span>
            <span>Cancel</span>
          </button>
          <button type="button" id="btn-picker-select" class="bg-accent hover:opacity-90 text-primary px-4 py-1.5 rounded text-xs font-semibold transition-opacity flex items-center space-x-1" disabled>
            <span class="material-symbols text-sm">check</span>
            <span>Select Folder</span>
          </button>
        </div>
      </div>
    `;

    // Append to modal card
    const cardEl = modal.querySelector('.bg-primary');
    cardEl.appendChild(pickerContainer);

    const closeBtn = pickerContainer.querySelector('#btn-picker-close');
    const cancelBtn = pickerContainer.querySelector('#btn-picker-cancel');
    const selectBtn = pickerContainer.querySelector('#btn-picker-select');
    const upBtn = pickerContainer.querySelector('#btn-picker-up');
    const currentPathEl = pickerContainer.querySelector('#picker-current-path');
    const dirsListEl = pickerContainer.querySelector('#picker-dirs-list');
    const subdirsListEl = pickerContainer.querySelector('#picker-subdirs-list');

    let currentPath = inputEl.value.trim() || '/';
    let selectedPath = currentPath;
    let directories = [];
    let subdirs = [];
    let isPosix = true;

    const closePicker = () => pickerContainer.remove();
    closeBtn.onclick = closePicker;
    cancelBtn.onclick = closePicker;

    selectBtn.onclick = () => {
      inputEl.value = selectedPath;
      closePicker();
    };

    function getLevel(pathStr) {
      if (!pathStr || pathStr === '/' || pathStr === '\\') return 0;
      const clean = pathStr.replace(/\\/g, '/');
      const parts = clean.split('/').filter(Boolean);
      return parts.length;
    }

    async function loadPath(path, lvl) {
      dirsListEl.innerHTML = '<div class="text-xs text-black-100 p-2">Loading...</div>';
      subdirsListEl.innerHTML = '';
      selectBtn.disabled = true;
      selectBtn.classList.add('opacity-50');

      try {
        const queryPath = path === '/' ? '' : path;
        const data = await request('GET', `/api/filesystem?path=${encodeURIComponent(queryPath)}&level=${lvl}`);
        isPosix = (data.posix !== false);
        directories = data.directories || [];
        renderDirs();
        updatePathDisplay();
        
        selectBtn.disabled = false;
        selectBtn.classList.remove('opacity-50');
      } catch (err) {
        if (path !== '/') {
          currentPath = '/';
          selectedPath = '/';
          loadPath('/', 0);
        } else {
          dirsListEl.innerHTML = `<div class="text-xs text-error p-2">Error: ${escapeHtml(err.message)}</div>`;
        }
      }
    }

    function getPathSegments(pathStr) {
      const segments = [];
      const clean = pathStr.replace(/\\/g, '/');
      const parts = clean.split('/').filter(Boolean);
      
      if (isPosix) {
        segments.push({ name: '/', path: '/' });
        let currentBuild = '';
        for (let i = 0; i < parts.length; i++) {
          currentBuild += '/' + parts[i];
          segments.push({ name: parts[i], path: currentBuild });
        }
      } else {
        let drive = '';
        let startIdx = 0;
        if (parts.length > 0 && /^[a-zA-Z]:$/.test(parts[0])) {
          drive = parts[0] + '\\';
          segments.push({ name: parts[0] + '\\', path: drive });
          startIdx = 1;
        } else {
          segments.push({ name: '\\', path: '\\' });
        }
        
        let currentBuild = drive;
        for (let i = startIdx; i < parts.length; i++) {
          if (currentBuild && !currentBuild.endsWith('\\')) {
            currentBuild += '\\';
          }
          currentBuild += parts[i];
          segments.push({ name: parts[i], path: currentBuild });
        }
      }
      return segments;
    }

    function updatePathDisplay() {
      currentPathEl.innerHTML = '';
      const segments = getPathSegments(currentPath);
      
      segments.forEach((seg, index) => {
        if (index > 0) {
          const sep = document.createElement('span');
          sep.className = 'text-black-300 mx-1 select-none font-mono';
          sep.textContent = isPosix ? '/' : '\\';
          currentPathEl.appendChild(sep);
        }
        
        const link = document.createElement('span');
        if (index === segments.length - 1) {
          link.className = 'font-semibold text-white font-mono';
          link.textContent = seg.name;
        } else {
          link.className = 'hover:text-accent cursor-pointer hover:underline transition-colors font-semibold text-gray-300 font-mono';
          link.textContent = seg.name;
          link.onclick = () => {
            currentPath = seg.path;
            selectedPath = seg.path;
            loadPath(currentPath, getLevel(currentPath));
          };
        }
        currentPathEl.appendChild(link);
      });

      if (currentPath === '/' || currentPath === '' || (!isPosix && /^[a-zA-Z]:\\?$/.test(currentPath))) {
        upBtn.disabled = true;
        upBtn.classList.add('opacity-50');
      } else {
        upBtn.disabled = false;
        upBtn.classList.remove('opacity-50');
      }
    }

    upBtn.onclick = () => {
      let parts;
      let newPath;
      if (isPosix) {
        parts = currentPath.split('/').filter(Boolean);
        parts.pop();
        newPath = '/' + parts.join('/');
      } else {
        parts = currentPath.split('\\').filter(Boolean);
        parts.pop();
        newPath = parts.join('\\');
        if (parts.length === 1 && /^[a-zA-Z]:$/.test(parts[0])) {
          newPath = parts[0] + '\\';
        }
      }
      currentPath = newPath;
      selectedPath = newPath;
      loadPath(currentPath, getLevel(currentPath));
    };

    function renderDirs() {
      dirsListEl.innerHTML = '';
      if (directories.length === 0) {
        dirsListEl.innerHTML = '<div class="text-xs text-black-100 p-2">No folders found.</div>';
        return;
      }

      directories.forEach(dir => {
        const item = document.createElement('button');
        item.type = 'button';
        item.className = 'w-full text-left flex items-center pl-2 pr-2 py-1.5 text-xs text-gray-300 hover:text-white hover:bg-black-400 transition-colors focus:outline-none rounded';
        
        const isSelected = dir.path === selectedPath;
        if (isSelected) {
          item.classList.remove('pl-2');
          item.classList.add('bg-black-500', 'text-white', 'font-semibold', 'border-l-2', 'border-accent', 'pl-1.5');
          selectBtn.disabled = false;
          selectBtn.classList.remove('opacity-50');
        }

        item.innerHTML = `
          <span class="material-symbols text-yellow-500 text-sm mr-1.5 flex items-center" style="font-variation-settings: 'FILL' 1;">folder</span>
          <span class="truncate grow font-mono text-left">${escapeHtml(dir.dirname)}</span>
          ${isSelected ? '<span class="material-symbols text-[14px] text-accent flex items-center">check</span>' : ''}
        `;

        item.onclick = (e) => {
          selectedPath = dir.path;
          Array.from(dirsListEl.children).forEach(child => {
            child.classList.remove('bg-black-500', 'text-white', 'font-semibold', 'border-l-2', 'border-accent', 'pl-1.5');
            child.classList.add('pl-2');
            const chk = child.querySelector('.text-accent');
            if (chk) chk.remove();
          });
          item.classList.remove('pl-2');
          item.classList.add('bg-black-500', 'text-white', 'font-semibold', 'border-l-2', 'border-accent', 'pl-1.5');
          const checkSpan = document.createElement('span');
          checkSpan.className = 'material-symbols text-[14px] text-accent flex items-center';
          checkSpan.textContent = 'check';
          item.appendChild(checkSpan);

          selectBtn.disabled = false;
          selectBtn.classList.remove('opacity-50');

          loadSubdirs(dir.path);
        };

        item.ondblclick = () => {
          currentPath = dir.path;
          selectedPath = dir.path;
          loadPath(currentPath, getLevel(currentPath));
        };

        dirsListEl.appendChild(item);
      });
    }

    async function loadSubdirs(path) {
      subdirsListEl.innerHTML = '<div class="text-xs text-black-100 p-2">Loading subfolders...</div>';
      try {
        const lvl = getLevel(path) + 1;
        const data = await request('GET', `/api/filesystem?path=${encodeURIComponent(path)}&level=${lvl}`);
        subdirs = data.directories || [];
        renderSubdirs();
      } catch (err) {
        subdirsListEl.innerHTML = `<div class="text-xs text-error p-2">Error: ${escapeHtml(err.message)}</div>`;
      }
    }

    function renderSubdirs() {
      subdirsListEl.innerHTML = '';
      if (subdirs.length === 0) {
        subdirsListEl.innerHTML = '<div class="text-xs text-black-100 p-2">No subfolders.</div>';
        return;
      }

      subdirs.forEach(dir => {
        const item = document.createElement('button');
        item.type = 'button';
        item.className = 'w-full text-left flex items-center px-2 py-1.5 text-xs text-gray-300 hover:text-white hover:bg-black-400 transition-colors focus:outline-none rounded';
        item.innerHTML = `
          <span class="material-symbols text-yellow-500/80 text-sm mr-1.5 flex items-center" style="font-variation-settings: 'FILL' 1;">folder</span>
          <span class="truncate grow font-mono text-left">${escapeHtml(dir.dirname)}</span>
          <span class="material-symbols text-xs text-gray-500 flex items-center">chevron_right</span>
        `;

        item.onclick = () => {
          currentPath = dir.path;
          selectedPath = dir.path;
          loadPath(currentPath, getLevel(currentPath));
        };

        subdirsListEl.appendChild(item);
      });
    }

    loadPath(currentPath, getLevel(currentPath));
  }

  // Add folder row helper
  function addFolderRow(val = '', id = '') {
    const row = document.createElement('div');
    row.className = 'flex items-center space-x-2';
    row.innerHTML = `
      <input type="text" class="lib-folder-path flex-grow bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm" placeholder="e.g. /path/to/media" value="${escapeHtml(val)}" data-id="${id}">
      <button type="button" class="btn-browse-folder text-accent hover:opacity-85 material-symbols text-lg focus:outline-none flex items-center justify-center p-1 hover:bg-white/5 rounded" title="Browse Folder">folder_open</button>
      <button type="button" class="btn-remove-folder-row text-error hover:text-red-400 material-symbols text-lg focus:outline-none flex items-center justify-center p-1 hover:bg-white/5 rounded" title="Remove Folder">delete</button>
    `;
    const input = row.querySelector('.lib-folder-path');
    row.querySelector('.btn-browse-folder').onclick = () => openFolderPicker(input);
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
    const btn = form.querySelector('button[type="submit"]');
    if (btn) {
      btn.disabled = true;
      btn.innerHTML = `<span class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary inline-block mr-1"></span><span>Saving...</span>`;
    }

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
      showToast('Please specify at least one valid folder path.', 'warning');
      if (btn) {
        btn.disabled = false;
        btn.innerHTML = `<span class="material-symbols text-sm">check</span><span>Save</span>`;
      }
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
      showToast('Failed to save library: ' + err.message, 'error');
      if (btn) {
        btn.disabled = false;
        btn.innerHTML = `<span class="material-symbols text-sm">check</span><span>Save</span>`;
      }
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

  // Chromecast visibility control
  const headerCastBtn = document.getElementById('header-cast-btn');
  const playerCastBtn = document.getElementById('player-cast-btn');
  const chromecastEnabled = settings.chromecastEnabled !== false;

  if (headerCastBtn) {
    if (chromecastEnabled) {
      headerCastBtn.classList.remove('hidden');
    } else {
      headerCastBtn.classList.add('hidden');
    }
  }
  if (playerCastBtn) {
    if (chromecastEnabled) {
      playerCastBtn.classList.remove('hidden');
    } else {
      playerCastBtn.classList.add('hidden');
    }
  }
}

// Register socket listeners for settings library scan animations
onEvent('library_scan_started', (libraryId) => {
  activeScans.add(libraryId);
  const btn = document.querySelector(`.btn-scan-lib[data-id="${libraryId}"]`);
  if (btn) {
    const icon = btn.querySelector('.material-symbols');
    if (icon) icon.classList.add('animate-spin');
  }
});

onEvent('library_scan_complete', (libraryId) => {
  activeScans.delete(libraryId);
  const btn = document.querySelector(`.btn-scan-lib[data-id="${libraryId}"]`);
  if (btn) {
    const icon = btn.querySelector('.material-symbols');
    if (icon) icon.classList.remove('animate-spin');
  }
});




