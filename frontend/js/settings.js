// frontend/js/settings.js (Proposed Implementation)
import { request, resolvePath } from './api.js';
import { getActiveLibraryId, getLibrariesList } from './library.js';

export async function loadSettings() {
  const container = document.getElementById('bookshelf');
  if (!container) return;

  // Set the toolbar view title
  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Settings & Administration';
  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = 'System Config';

  // Render settings structure with tabs
  container.innerHTML = `
    <div class="max-w-4xl mx-auto p-4">
      <div class="flex border-b border-black-400 mb-6" id="settings-tabs">
        <button class="px-4 py-2 border-b-2 border-accent text-accent font-semibold focus:outline-none" data-tab="users">Users</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="server">Server Settings</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="auth">Authentication (OIDC)</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="backups">Backups</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="providers">Metadata Providers</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="upload">Upload Media</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="apikeys">API Keys</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="listening-sessions">Listening Sessions</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="logs">Logs</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="notifications">Notifications</button>
      </div>

      <!-- Tab Contents -->
      <div id="settings-tab-content">
        <div id="tab-users" class="space-y-6"></div>
        <div id="tab-server" class="space-y-6 hidden"></div>
        <div id="tab-auth" class="space-y-6 hidden"></div>
        <div id="tab-backups" class="space-y-6 hidden"></div>
        <div id="tab-providers" class="space-y-6 hidden"></div>
        <div id="tab-upload" class="space-y-6 hidden"></div>
        <div id="tab-apikeys" class="space-y-6 hidden"></div>
        <div id="tab-listening-sessions" class="space-y-6 hidden"></div>
        <div id="tab-logs" class="space-y-6 hidden"></div>
        <div id="tab-notifications" class="space-y-6 hidden"></div>
      </div>
    </div>
  `;

  // Attach tab switcher click handlers
  const tabs = document.querySelectorAll('#settings-tabs button');
  tabs.forEach(tab => {
    tab.onclick = () => {
      tabs.forEach(t => {
        t.classList.remove('border-accent', 'text-accent', 'font-semibold');
        t.classList.add('border-transparent', 'text-black-50');
      });
      tab.classList.add('border-accent', 'text-accent', 'font-semibold');
      tab.classList.remove('border-transparent', 'text-black-50');

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
    renderServerSettingsTab(),
    renderAuthSettingsTab(),
    renderBackupsTab(),
    renderProvidersTab(),
    renderUploadTab(),
    renderApiKeysTab(),
    renderListeningSessionsTab(),
    renderLogsTab(),
    renderNotificationsTab()
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

          <div class="flex flex-col space-y-2 pt-2">
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-metadata-cover-with-item" ${settings.metadataCoverWithItem ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Embed cover image in item metadata folder</span>
            </label>
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-metadata-markdown-with-item" ${settings.metadataMarkdownWithItem ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Save metadata as markdown alongside media files</span>
            </label>
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-sorting-ignore-prefix" ${settings.sortingIgnorePrefix !== false ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Ignore title prefixes ("The", "A", "An", etc.) when sorting</span>
            </label>
          </div>
        </div>

        <hr class="border-black-400">

        <!-- Category 2: Scanner Settings -->
        <div class="space-y-4">
          <h4 class="text-md font-semibold text-accent">Scanner Settings</h4>

          <div class="flex flex-col space-y-2">
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-scanner-parse-subtitles" ${settings.scannerParseSubtitles !== false ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Parse subtitles from folders/filenames</span>
            </label>
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-scanner-find-covers" ${settings.scannerFindCovers !== false ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
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

          <div class="flex flex-col space-y-2 pt-2">
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-scanner-prefer-matched-metadata" ${settings.scannerPreferMatchedMetadata ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Prefer matched metadata over embedded tags</span>
            </label>
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-watch-library-changes" ${settings.watchLibraryChanges !== false ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Watch library folders for changes</span>
            </label>
          </div>
        </div>

        <hr class="border-black-400">

        <!-- Category 3: Web Client Settings -->
        <div class="space-y-4">
          <h4 class="text-md font-semibold text-accent">Web Client Settings</h4>
          
          <div class="flex flex-col space-y-2">
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-chromecast-enabled" ${settings.chromecastEnabled ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Enable Chromecast support</span>
            </label>
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-allow-iframe" ${settings.allowIframe ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Allow embedding app in an iframe</span>
            </label>
          </div>
        </div>

        <hr class="border-black-400">

        <!-- Category 4: Display Settings -->
        <div class="space-y-4">
          <h4 class="text-md font-semibold text-accent">Display Settings</h4>

          <div class="flex flex-col space-y-2">
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-home-page-bookshelf-view" ${settings.homePageBookshelfView ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Show Home Page in Bookshelf View</span>
            </label>
            <label class="flex items-center space-x-2 cursor-pointer text-sm">
              <input type="checkbox" id="setting-library-bookshelf-view" ${settings.libraryBookshelfView ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Show Library Page in Bookshelf View</span>
            </label>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
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
          
          allowedCorsOrigins: allowedCorsOrigins
        };

        const res = await request('PATCH', '/api/settings', payload);
        if (res && res.serverSettings) {
          window.serverSettings = res.serverSettings;
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
            <label class="flex items-center space-x-2 cursor-pointer">
              <input type="checkbox" id="auth-method-local" value="local" ${activeMethods.includes('local') ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
              <span>Local Accounts</span>
            </label>
            <label class="flex items-center space-x-2 cursor-pointer">
              <input type="checkbox" id="auth-method-openid" value="openid" ${activeMethods.includes('openid') ? 'checked' : ''} class="rounded text-accent focus:ring-accent bg-black-500 border-black-300">
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
          
          <div class="flex items-center space-x-6 pt-2">
            <label class="flex items-center space-x-2 cursor-pointer text-xs">
              <input type="checkbox" id="oidc-autolaunch" ${auth.authOpenIDAutoLaunch ? 'checked' : ''} class="rounded text-accent bg-black-500 border-black-300">
              <span>Auto-Launch OpenID (Skips login form)</span>
            </label>
            <label class="flex items-center space-x-2 cursor-pointer text-xs">
              <input type="checkbox" id="oidc-autoregister" ${auth.authOpenIDAutoRegister ? 'checked' : ''} class="rounded text-accent bg-black-500 border-black-300">
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
    const backupPayload = await request('GET', '/api/backups');
    const backups = backupPayload.backups || [];
    const location = backupPayload.backupLocation || '';

    container.innerHTML = `
      <div class="bg-primary border border-black-300 p-6 rounded-md space-y-6">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">Backup Path & Trigger</h3>
        
        <form id="backup-path-form" class="flex items-end space-x-4">
          <div class="flex-grow">
            <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Backups Storage Directory</label>
            <input type="text" id="backup-location-path" value="${escapeHtml(location)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
          </div>
          <button type="submit" class="bg-black-400 hover:bg-black-300 border border-black-300 text-white font-medium px-4 py-2 rounded transition-colors">Change Path</button>
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
              <tr class="border-b border-black-400 text-xs text-black-100 uppercase tracking-wider">
                <th class="py-2.5">Date</th>
                <th class="py-2.5">Filename</th>
                <th class="py-2.5">Size</th>
                <th class="py-2.5 text-right">Actions</th>
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
    tr.className = 'hover:bg-black-500/20';

    const sizeFormatted = (b.fileSize / (1024 * 1024)).toFixed(2) + ' MB';
    
    // Download link
    const downloadUrl = resolvePath(`/api/backups/${b.id}/download?token=${localStorage.getItem('token')}`);

    tr.innerHTML = `
      <td class="py-3 font-medium text-white">${b.datePretty}</td>
      <td class="py-3 font-mono text-xs">${escapeHtml(b.filename)}</td>
      <td class="py-3">${sizeFormatted}</td>
      <td class="py-3 text-right space-x-2">
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
              <tr class="border-b border-black-400 text-xs text-black-100 uppercase tracking-wider">
                <th class="py-2.5">Name</th>
                <th class="py-2.5">Media Type</th>
                <th class="py-2.5">URL</th>
                <th class="py-2.5 text-right">Action</th>
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
    tr.className = 'hover:bg-black-500/20';

    tr.innerHTML = `
      <td class="py-3 font-semibold text-white">${escapeHtml(p.name)}</td>
      <td class="py-3 uppercase text-xs">${escapeHtml(p.mediaType)}</td>
      <td class="py-3 font-mono text-xs truncate max-w-xs">${escapeHtml(p.url)}</td>
      <td class="py-3 text-right">
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
              <tr class="border-b border-black-400 text-xs text-black-100 uppercase tracking-wider font-semibold">
                <th class="py-2.5">Username</th>
                <th class="py-2.5">Account Type</th>
                <th class="py-2.5">Last Seen</th>
                <th class="py-2.5">Created At</th>
                <th class="py-2.5 text-right">Actions</th>
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
    tr.className = 'hover:bg-black-500/20';

    const lastSeenFormatted = u.lastSeen ? window.formatDateTime(u.lastSeen) : 'Never';
    const createdAtFormatted = u.createdAt ? window.formatDateTime(u.createdAt) : 'Unknown';
    
    let typeDisplay = u.type || 'user';
    if (typeDisplay === 'root') typeDisplay = 'Root Admin';
    else if (typeDisplay === 'admin') typeDisplay = 'Admin';
    else typeDisplay = 'User';

    const canDelete = u.type !== 'root' && u.id !== currentUser.id;
    const canEdit = currentUser.type === 'root' || (currentUser.type === 'admin' && u.type !== 'root');

    tr.innerHTML = `
      <td class="py-3 font-semibold text-white flex items-center space-x-2">
        <span>${escapeHtml(u.username)}</span>
        ${u.isActive ? '' : '<span class="bg-red-900/50 text-red-200 text-[10px] px-1.5 py-0.5 rounded font-normal uppercase">Inactive</span>'}
        ${u.hasOpenIDLink ? '<span class="bg-blue-900/50 text-blue-200 text-[10px] px-1.5 py-0.5 rounded font-normal uppercase">OIDC</span>' : ''}
      </td>
      <td class="py-3 text-xs capitalize">${typeDisplay}</td>
      <td class="py-3 text-xs text-black-100">${lastSeenFormatted}</td>
      <td class="py-3 text-xs text-black-100">${createdAtFormatted}</td>
      <td class="py-3 text-right space-x-2">
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
            <label class="flex items-center space-x-2 text-sm cursor-pointer ${isEdit && user.type === 'root' ? 'opacity-50 pointer-events-none' : ''}">
              <input type="checkbox" id="user-isactive" ${!isEdit || user.isActive ? 'checked' : ''} ${isEdit && user.type === 'root' ? 'disabled' : ''} class="rounded text-accent bg-black-600 border-black-300">
              <span>Account is Active</span>
            </label>
          </div>
        </div>

        <!-- Permissions Section -->
        <div class="border-t border-black-400 pt-3 space-y-3">
          <h4 class="text-xs font-semibold text-accent uppercase tracking-wider">Permissions</h4>
          
          <div class="flex flex-col space-y-2">
            <label class="flex items-center space-x-2 text-xs cursor-pointer">
              <input type="checkbox" id="perm-download" ${perms.download !== false ? 'checked' : ''} class="rounded text-accent bg-black-600 border-black-300">
              <span>Allow downloading files</span>
            </label>
            <label class="flex items-center space-x-2 text-xs cursor-pointer">
              <input type="checkbox" id="perm-explicit" ${perms.accessExplicitContent ? 'checked' : ''} class="rounded text-accent bg-black-600 border-black-300">
              <span>Allow explicit content access</span>
            </label>
            <label class="flex items-center space-x-2 text-xs cursor-pointer">
              <input type="checkbox" id="perm-all-libraries" ${perms.accessAllLibraries !== false ? 'checked' : ''} class="rounded text-accent bg-black-600 border-black-300">
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
          
          <div class="flex flex-col space-y-2">
            <label class="flex items-center space-x-2 text-xs cursor-pointer">
              <input type="checkbox" id="perm-all-tags" ${accessAllTags ? 'checked' : ''} class="rounded text-accent bg-black-600 border-black-300">
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
              <tr class="border-b border-black-400 text-black-100 text-xs uppercase">
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
    const sessions = sessionsResp.sessions || [];

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
              <tr class="border-b border-black-400 text-black-100 text-xs uppercase">
                <th class="px-4 py-3">User</th>
                <th class="px-4 py-3">Item</th>
                <th class="px-4 py-3">Play Method</th>
                <th class="px-4 py-3">Device Info</th>
                <th class="px-4 py-3">Time Listened</th>
                <th class="px-4 py-3">Last Position/Last Time</th>
                <th class="px-4 py-3">Last Updated</th>
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
    select.onchange = async () => {
      const tbody = document.getElementById('sessions-list-rows');
      if (tbody) {
        tbody.innerHTML = `
          <tr>
            <td colspan="7" class="px-4 py-8 text-center">
              <div class="animate-spin rounded-full h-6 w-6 border-b-2 border-accent mx-auto"></div>
            </td>
          </tr>
        `;
      }
      try {
        const selectedUserId = select.value;
        let url = '/api/playback-sessions';
        if (selectedUserId) {
          url += `?userId=${selectedUserId}`;
        }
        const res = await request('GET', url);
        renderListeningSessionsListRows(res.sessions || []);
      } catch (err) {
        if (tbody) {
          tbody.innerHTML = `
            <tr>
              <td colspan="7" class="px-4 py-8 text-center text-red-500">
                Failed to load sessions: ${escapeHtml(err.message)}
              </td>
            </tr>
          `;
        }
      }
    };

    renderListeningSessionsListRows(sessions);
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
        <td colspan="7" class="px-4 py-8 text-center text-black-100">
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

    tr.innerHTML = `
      <td class="px-4 py-3 font-semibold text-white">${escapeHtml(session.username || 'Unknown')}</td>
      <td class="px-4 py-3 text-black-50 font-medium">${escapeHtml(session.title || 'Unknown')}</td>
      <td class="px-4 py-3 text-black-100"><span class="px-2 py-0.5 rounded text-xs bg-black-400 font-mono">${escapeHtml(session.playMethod || 'HLS')}</span></td>
      <td class="px-4 py-3 text-black-100">${escapeHtml(session.deviceInfo || 'Web Client')}</td>
      <td class="px-4 py-3 text-black-100 font-mono text-xs">${escapeHtml(timeListenedFormatted)}</td>
      <td class="px-4 py-3 text-black-100 font-mono text-xs">${escapeHtml(lastTimeFormatted)}</td>
      <td class="px-4 py-3 text-black-100">${escapeHtml(updatedAtFormatted)}</td>
    `;

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

    // Initial log display
    displayLogs();

    // 4. Setup Event Listeners
    logLevelSelect.onchange = async () => {
      const val = parseInt(logLevelSelect.value, 10);
      const prevVal = currentSelectedLevel;
      currentSelectedLevel = val;
      displayLogs();

      try {
        await request('PATCH', '/api/settings', { logLevel: val });
      } catch (err) {
        alert('Failed to save log level on server: ' + err.message);
        currentSelectedLevel = prevVal;
        logLevelSelect.value = prevVal;
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
              <tr class="border-b border-black-400 text-black-100 text-xs uppercase">
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
          <label class="flex items-center space-x-2 text-sm cursor-pointer">
            <input type="checkbox" id="notif-enabled" checked class="rounded text-accent bg-black-600 border-black-300">
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
