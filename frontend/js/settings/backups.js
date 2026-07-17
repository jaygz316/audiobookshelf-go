import { request, resolvePath } from '../api.js';
import { showToast } from '../toast.js';

function escapeHtml(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

export async function renderBackupsTab() {
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
          <button type="submit" class="bg-black-400 hover:bg-black-300 border border-black-300 text-white font-medium px-4 py-2 rounded transition-colors flex items-center space-x-1.5 text-sm">
            <span class="material-symbols text-sm">save</span>
            <span>Change Path</span>
          </button>
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
              <button type="submit" class="bg-black-400 hover:bg-black-300 border border-black-300 text-white font-medium px-4 py-2 rounded transition-colors w-full md:w-auto flex items-center justify-center space-x-1.5 text-sm">
                <span class="material-symbols text-sm">save</span>
                <span>Save Schedule</span>
              </button>
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
      const btn = document.querySelector('#backup-schedule-form button[type="submit"]');
      if (btn) {
        btn.disabled = true;
        btn.innerHTML = `<span class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary inline-block mr-1.5"></span><span>Saving...</span>`;
      }
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
        showToast('Backup schedule updated successfully!', 'success');
        renderBackupsTab(); // reload
      } catch (err) {
        showToast('Failed to update backup schedule: ' + err.message, 'error');
        if (btn) {
          btn.disabled = false;
          btn.innerHTML = `<span class="material-symbols text-sm">save</span><span>Save Schedule</span>`;
        }
      }
    };

    document.getElementById('backup-path-form').onsubmit = async (e) => {
      e.preventDefault();
      const btn = document.querySelector('#backup-path-form button[type="submit"]');
      if (btn) {
        btn.disabled = true;
        btn.innerHTML = `<span class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary inline-block mr-1.5"></span><span>Saving...</span>`;
      }
      try {
        const path = document.getElementById('backup-location-path').value;
        await request('PATCH', '/api/backups/path', { path });
        showToast('Backup path updated successfully!', 'success');
        renderBackupsTab(); // reload
      } catch (err) {
        showToast('Failed to update backup path: ' + err.message, 'error');
        if (btn) {
          btn.disabled = false;
          btn.innerHTML = `<span class="material-symbols text-sm">save</span><span>Change Path</span>`;
        }
      }
    };

    document.getElementById('create-backup-btn').onclick = async () => {
      const btn = document.getElementById('create-backup-btn');
      btn.disabled = true;
      btn.innerHTML = `<span class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary mr-1.5"></span><span>Creating...</span>`;
      try {
        const res = await request('POST', '/api/backups');
        renderBackupsListRows(res.backups || []);
        showToast('Backup created successfully!', 'success');
      } catch (err) {
        showToast('Failed to create backup: ' + err.message, 'error');
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
        showToast('Backup uploaded successfully!', 'success');
      } catch (err) {
        showToast('Upload failed: ' + err.message, 'error');
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
        <button class="apply-btn bg-emerald-800 hover:bg-emerald-700 text-emerald-100 text-xs font-semibold px-2.5 py-1 rounded inline-flex items-center space-x-1" data-id="${b.id}">
          <span class="material-symbols text-sm">settings_backup_restore</span>
          <span>Restore</span>
        </button>
        <a href="${downloadUrl}" class="inline-block bg-black-400 hover:bg-black-300 text-white text-xs font-semibold px-2.5 py-1 rounded inline-flex items-center space-x-1">
          <span class="material-symbols text-sm">download</span>
          <span>Download</span>
        </a>
        <button class="delete-btn bg-red-900 hover:bg-red-800 text-red-200 text-xs font-semibold px-2.5 py-1 rounded inline-flex items-center space-x-1" data-id="${b.id}">
          <span class="material-symbols text-sm">delete</span>
          <span>Delete</span>
        </button>
      </td>
    `;

    // Bind triggers
    tr.querySelector('.apply-btn').onclick = async () => {
      const confirmed = await window.showConfirm(
        'Restore Backup',
        `Are you absolutely sure you want to restore the backup from ${b.datePretty}? This will disconnect current sessions, overwrite the database, and trigger a server reload.`,
        'Restore',
        'Cancel'
      );
      if (!confirmed) {
        return;
      }
      try {
        await request('POST', `/api/backups/${b.id}/apply`);
        showToast('Backup applied successfully! Page will reload.', 'success');
        window.location.reload();
      } catch (err) {
        showToast('Restore failed: ' + err.message, 'error');
      }
    };

    tr.querySelector('.delete-btn').onclick = async () => {
      const confirmed = await window.showConfirm(
        'Delete Backup',
        `Delete backup file ${b.filename}?`,
        'Delete',
        'Cancel'
      );
      if (!confirmed) return;
      try {
        const res = await request('DELETE', `/api/backups/${b.id}`);
        renderBackupsListRows(res.backups || []);
        showToast('Backup deleted successfully.', 'success');
      } catch (err) {
        showToast('Delete failed: ' + err.message, 'error');
      }
    };

    tbody.appendChild(tr);
  });
}
