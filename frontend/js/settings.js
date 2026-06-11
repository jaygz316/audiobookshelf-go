// frontend/js/settings.js (Proposed Implementation)
import { request, resolvePath } from './api.js';
import { getActiveLibraryId } from './library.js';

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
        <button class="px-4 py-2 border-b-2 border-accent text-accent font-semibold focus:outline-none" data-tab="server">Server Settings</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="auth">Authentication (OIDC)</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="backups">Backups</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="providers">Metadata Providers</button>
        <button class="px-4 py-2 border-b-2 border-transparent hover:text-white text-black-50 focus:outline-none" data-tab="upload">Upload Media</button>
      </div>

      <!-- Tab Contents -->
      <div id="settings-tab-content">
        <div id="tab-server" class="space-y-6"></div>
        <div id="tab-auth" class="space-y-6 hidden"></div>
        <div id="tab-backups" class="space-y-6 hidden"></div>
        <div id="tab-providers" class="space-y-6 hidden"></div>
        <div id="tab-upload" class="space-y-6 hidden"></div>
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
    renderServerSettingsTab(),
    renderAuthSettingsTab(),
    renderBackupsTab(),
    renderProvidersTab(),
    renderUploadTab()
  ]);
}

async function renderServerSettingsTab() {
  const container = document.getElementById('tab-server');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const settings = await request('GET', '/api/settings');
    const prefixes = settings.sortingPrefixes || ['a', 'the', 'an'];

    container.innerHTML = `
      <form id="server-settings-form" class="space-y-6 bg-primary border border-black-300 p-6 rounded-md">
        <h3 class="text-lg font-semibold border-b border-black-400 pb-2">General Server Settings</h3>
        
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

        <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity">Save General Settings</button>
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
    document.getElementById('server-settings-form').onsubmit = async (e) => {
      e.preventDefault();
      try {
        const payload = {
          language: document.getElementById('setting-language').value,
          backupsToKeep: parseInt(document.getElementById('setting-backups-to-keep').value, 10)
        };
        await request('PATCH', '/api/settings', payload);
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
