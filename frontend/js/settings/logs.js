import { request } from '../api.js';
import { showToast } from '../toast.js';
import { onEvent, offEvent, sendEvent } from '../socket.js';
import { logout } from '../auth.js';

let currentSessions = [];
let selectedUserIdFilter = '';
let searchFilter = '';
let methodFilter = '';
let tasksPollInterval = null;

function getFilteredSessions() {
  let filtered = currentSessions;
  if (selectedUserIdFilter) {
    filtered = filtered.filter(s => s.userId === selectedUserIdFilter);
  }
  if (methodFilter) {
    filtered = filtered.filter(s => (s.playMethod || '').toLowerCase() === methodFilter.toLowerCase());
  }
  if (searchFilter) {
    const q = searchFilter.toLowerCase();
    filtered = filtered.filter(s => 
      (s.username || '').toLowerCase().includes(q) ||
      (s.title || '').toLowerCase().includes(q) ||
      (s.deviceInfo || '').toLowerCase().includes(q)
    );
  }
  return filtered;
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

// Render the Listening Sessions settings tab
export async function renderListeningSessionsTab() {
  const container = document.getElementById('tab-listening-sessions');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const [usersResp, sessionsResp] = await Promise.all([
      request('GET', '/api/users'),
      request('GET', '/api/playback-sessions')
    ]);
    const users = usersResp.users || [];
    currentSessions = sessionsResp.sessions || [];
    selectedUserIdFilter = '';
    searchFilter = '';
    methodFilter = '';

    container.innerHTML = `
      <div class="space-y-4">
        <div class="flex flex-col sm:flex-row gap-4 justify-between items-start sm:items-center bg-black-500/10 p-3 rounded-md border border-black-400/40">
          <h3 class="text-lg font-semibold text-white">Listening Sessions</h3>
          <div class="flex flex-wrap items-center gap-3">
            <div class="flex items-center space-x-2">
              <label for="search-session" class="text-xs text-black-100 uppercase tracking-wider font-semibold">Search:</label>
              <input type="text" id="search-session" placeholder="Search sessions..." class="bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm w-44 font-semibold">
            </div>
            <div class="flex items-center space-x-2">
              <label for="filter-session-user" class="text-xs text-black-100 uppercase tracking-wider font-semibold">User:</label>
              <select id="filter-session-user" class="bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm font-semibold">
                <option value="">All Users</option>
                ${users.map(u => `<option value="${u.id}">${escapeHtml(u.username)}</option>`).join('')}
              </select>
            </div>
            <div class="flex items-center space-x-2">
              <label for="filter-session-method" class="text-xs text-black-100 uppercase tracking-wider font-semibold">Method:</label>
              <select id="filter-session-method" class="bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm font-semibold">
                <option value="">All</option>
                <option value="HLS">HLS</option>
                <option value="Direct Play">Direct Play</option>
              </select>
            </div>
          </div>
        </div>

        <div class="border border-black-300 rounded-md bg-primary overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-black-400/60 text-black-100 text-[10px] uppercase tracking-wider font-bold">
                <th class="px-4 py-3">User</th>
                <th class="px-4 py-3">Item</th>
                <th class="px-4 py-3">Method</th>
                <th class="px-4 py-3">Device</th>
                <th class="px-4 py-3">Time</th>
                <th class="px-4 py-3">Position</th>
                <th class="px-4 py-3">Updated</th>
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

    const searchInput = container.querySelector('#search-session');
    searchInput.oninput = () => {
      searchFilter = searchInput.value;
      renderListeningSessionsListRows(getFilteredSessions());
    };

    const methodSelect = container.querySelector('#filter-session-method');
    methodSelect.onchange = () => {
      methodFilter = methodSelect.value;
      renderListeningSessionsListRows(getFilteredSessions());
    };

    renderListeningSessionsListRows(getFilteredSessions());
  } catch (err) {
    container.innerHTML = `<div class="text-error text-center py-4 font-semibold">Failed to load listening sessions: ${escapeHtml(err.message)}</div>`;
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
    tr.className = 'hover:bg-black-500/20 transition-colors border-b border-black-400/20';

    const timeListenedFormatted = formatSessionTime(session.timeListened);
    const lastTimeFormatted = formatSessionTime(session.lastTime);
    const updatedAtFormatted = session.updatedAt ? (window.formatDateTime ? window.formatDateTime(session.updatedAt) : session.updatedAt) : 'Unknown';

    // Verify current user permissions to show Close button
    const curUser = window.currentUser || {};
    const canClose = curUser.type === 'root' || curUser.type === 'admin' || curUser.id === session.userId;

    let actionsHtml = '';
    if (canClose) {
      actionsHtml = `
        <button class="close-session-btn bg-black-500/50 hover:bg-red-900/60 border border-black-300 hover:border-red-500/50 text-black-100 hover:text-white text-xs font-semibold px-3 py-1 rounded inline-flex items-center space-x-1.5 transition-all cursor-pointer" data-id="${session.id}">
          <span class="material-symbols text-sm">close</span>
          <span>Close Session</span>
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
        const confirmed = await window.showConfirm(
          'Close Playback Session',
          `Are you sure you want to close this playback session for ${session.username || 'user'}?`,
          'Close',
          'Cancel'
        );
        if (confirmed) {
          try {
            await request('DELETE', `/api/playback-sessions/${session.id}/`);
            // Note: socket listener will automatically remove it and re-render.
            // But we can also remove it locally right away for an instant UI update:
            currentSessions = currentSessions.filter(s => s.id !== session.id);
            renderListeningSessionsListRows(getFilteredSessions());
            showToast('Playback session closed successfully.', 'success');
          } catch (err) {
            showToast('Failed to close playback session: ' + err.message, 'error');
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

export async function renderLoginSessionsTab() {
  const container = document.getElementById('tab-login-sessions');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const usersResp = await request('GET', '/api/users');
    const users = usersResp.users || [];
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
    container.innerHTML = `<div class="text-error text-center py-4">Failed to load active login sessions: ${escapeHtml(err.message)}</div>`;
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
        <button class="revoke-login-session-btn bg-red-900/40 hover:bg-red-900/60 border border-red-500/30 text-error hover:text-white hover:border-red-500/50 text-xs font-semibold px-2.5 py-1 rounded inline-flex items-center space-x-1 transition-colors cursor-pointer" data-id="${session.id}">
          <span class="material-symbols text-sm">close</span>
          <span>Revoke</span>
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

        const title = session.isCurrent ? 'Revoke Current Session' : 'Revoke Login Session';
        const confirmed = await window.showConfirm(
          title,
          confirmMsg,
          'Revoke',
          'Cancel'
        );
        if (confirmed) {
          try {
            await request('DELETE', `/api/users/${userId}/sessions/${session.id}`);
            if (session.isCurrent) {
              await logout();
              window.location.reload();
            } else {
              loadAndRenderLoginSessions(userId);
            }
            showToast('Login session revoked successfully.', 'success');
          } catch (err) {
            showToast('Failed to revoke session: ' + err.message, 'error');
          }
        }
      };

      tbody.appendChild(tr);
    });

  } catch (err) {
    tbody.innerHTML = `
      <tr>
        <td colspan="5" class="px-4 py-8 text-center text-error">
          Failed to load sessions: ${escapeHtml(err.message)}
        </td>
      </tr>
    `;
  }
}

export async function renderLogsTab() {
  const container = document.getElementById('tab-logs');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const [settings, loggerData] = await Promise.all([
      request('GET', '/api/settings'),
      request('GET', '/api/logger-data')
    ]);

    const activeLogLevel = (settings && settings.logLevel !== undefined) ? parseInt(settings.logLevel, 10) : 2;
    const logEntries = (loggerData && loggerData.currentDailyLogs) || [];

    container.innerHTML = `
      <div class="bg-primary border border-black-300 p-6 rounded-md space-y-6">
        <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4 border-b border-black-400 pb-4">
          <div>
            <h3 class="text-lg font-semibold text-white">Server Logs</h3>
            <p class="text-xs text-black-100 mt-1">View system logs and adjust log verbosity.</p>
          </div>
          <div class="flex flex-col sm:flex-row sm:items-center gap-3">
            <div class="flex items-center space-x-2">
              <label for="log-level-select" class="text-xs text-black-100 uppercase tracking-wider whitespace-nowrap">Log Level:</label>
              <select id="log-level-select" class="bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
                <option value="1" ${activeLogLevel === 1 ? 'selected' : ''}>DEBUG</option>
                <option value="2" ${activeLogLevel === 2 ? 'selected' : ''}>INFO</option>
                <option value="3" ${activeLogLevel === 3 ? 'selected' : ''}>WARN</option>
                <option value="4" ${activeLogLevel === 4 ? 'selected' : ''}>ERROR</option>
              </select>
            </div>
            
            <div class="flex items-center space-x-2">
              <input type="text" id="log-search-input" placeholder="Search logs..." class="bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm w-full sm:w-48">
            </div>
          </div>
        </div>

        <div id="logs-console" class="bg-black-900 border border-black-400 rounded p-4 font-mono text-xs overflow-y-auto h-[500px] space-y-1 text-black-50 scrollbar-thin">
        </div>
      </div>
    `;

    const logConsole = container.querySelector('#logs-console');
    const logLevelSelect = container.querySelector('#log-level-select');
    const logSearchInput = container.querySelector('#log-search-input');

    let currentSelectedLevel = parseInt(logLevelSelect.value, 10);
    let currentSearchQuery = '';

    function displayLogs() {
      logConsole.innerHTML = '';
      
      const filtered = logEntries.filter(entry => {
        const matchesLevel = entry.level >= currentSelectedLevel;
        
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

      const limitedLogs = filtered.slice(-1000);
      const fragment = document.createDocumentFragment();
      limitedLogs.forEach(entry => {
        const div = document.createElement('div');
        div.className = 'whitespace-pre-wrap leading-relaxed py-0.5 border-b border-black-400/10 last:border-b-0';
        
        let levelColorClass = 'text-black-100';
        if (entry.level === 2) {
          levelColorClass = 'text-success';
        } else if (entry.level === 3) {
          levelColorClass = 'text-warning';
        } else if (entry.level === 4) {
          levelColorClass = 'text-error';
        }

        const timestamp = escapeHtml(String(entry.timestamp || ''));
        const levelName = escapeHtml(String(entry.levelName || 'INFO'));
        const message = escapeHtml(String(entry.message || ''));

        div.innerHTML = `<span class="text-black-100">[${timestamp}]</span> <span class="${levelColorClass} font-semibold">[${levelName}]</span> <span class="text-white">${message}</span>`;
        fragment.appendChild(div);
      });
      logConsole.appendChild(fragment);

      logConsole.scrollTop = logConsole.scrollHeight;
    }

    sendEvent('set_log_listener', activeLogLevel);

    const logSocketCallback = (logMsg) => {
      logEntries.push(logMsg);
      if (logEntries.length > 2000) {
        logEntries.shift();
      }
      displayLogs();
    };

    onEvent('log', logSocketCallback);

    window.cleanupSettings = () => {
      sendEvent('remove_log_listener');
      offEvent('log', logSocketCallback);
    };

    displayLogs();

    logLevelSelect.onchange = async () => {
      const val = parseInt(logLevelSelect.value, 10);
      const prevVal = currentSelectedLevel;
      currentSelectedLevel = val;
      displayLogs();

      sendEvent('set_log_listener', val);

      try {
        await request('PATCH', '/api/settings', { logLevel: val });
        showToast('Log level updated successfully.', 'success');
      } catch (err) {
        showToast('Failed to save log level on server: ' + err.message, 'error');
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
    container.innerHTML = `<p class="text-error text-sm">Failed to load logs: ${escapeHtml(err.message)}</p>`;
  }
}

export async function renderTasksTab() {
  const container = document.getElementById('tab-tasks');
  if (!container) return;

  container.innerHTML = `
    <div class="space-y-4 text-left">
      <div class="flex justify-between items-center">
        <div>
          <h3 class="text-lg font-semibold text-white">Active Tasks & Downloads</h3>
          <p class="text-xs text-black-100 font-medium">Monitor and manage real-time episode downloads and background operations.</p>
        </div>
        <button id="cancel-all-tasks-btn" class="bg-red-900/40 hover:bg-red-900/60 border border-red-500/30 text-error hover:text-white hover:border-red-500/50 font-bold px-3 py-1.5 rounded text-xs transition-colors flex items-center gap-1 focus:outline-none cursor-pointer">
          <span class="material-symbols text-sm">cancel</span>
          <span>Cancel All Tasks</span>
        </button>
      </div>

      <div class="border border-black-300 rounded-md bg-primary overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead>
            <tr class="border-b border-black-400/60 text-black-100 text-xs uppercase tracking-wider font-semibold">
              <th class="px-4 py-3">Podcast</th>
              <th class="px-4 py-3">Episode / Action</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3">Progress</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody id="tasks-list-rows" class="divide-y divide-black-400">
            <tr>
              <td colspan="5" class="px-4 py-8 text-center text-black-100 text-xs">Loading tasks...</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  `;

  const cancelAllBtn = container.querySelector('#cancel-all-tasks-btn');
  cancelAllBtn.onclick = async () => {
    const confirmed = await window.showConfirm(
      'Cancel All Tasks',
      'Are you sure you want to cancel all running and queued tasks?',
      'Cancel Tasks',
      'Cancel'
    );
    if (confirmed) {
      try {
        await request('POST', '/api/tasks/cancel-all');
        showToast('All tasks cancelled', 'success');
        updateTasksList();
      } catch (err) {
        showToast('Failed to cancel tasks: ' + err.message, 'error');
      }
    }
  };

  if (tasksPollInterval) clearInterval(tasksPollInterval);
  updateTasksList();
  tasksPollInterval = setInterval(updateTasksList, 2000);
}

async function updateTasksList() {
  const tbody = document.getElementById('tasks-list-rows');
  const tabTasks = document.getElementById('tab-tasks');
  if (!tbody || !tabTasks || tabTasks.classList.contains('hidden')) {
    if (tasksPollInterval) {
      clearInterval(tasksPollInterval);
      tasksPollInterval = null;
    }
    return;
  }

  try {
    const data = await request('GET', '/api/tasks');
    const tasks = data.tasks || [];

    if (tasks.length === 0) {
      tbody.innerHTML = `
        <tr>
          <td colspan="5" class="px-4 py-8 text-center text-black-100 text-xs">No active tasks or downloads.</td>
        </tr>
      `;
      return;
    }

    tbody.innerHTML = tasks.map(task => {
      let progressPct = 0;
      if (task.bytesTotal > 0) {
        progressPct = Math.round((task.bytesDownloaded / task.bytesTotal) * 100);
      }
      
      const downloadedMb = (task.bytesDownloaded / (1024 * 1024)).toFixed(1);
      const totalMb = (task.bytesTotal / (1024 * 1024)).toFixed(1);
      
      let statusBadge = '';
      if (task.status === 'running') {
        statusBadge = `<span class="bg-info/10 text-info border border-info/30 px-1.5 py-0.5 rounded text-[10px] uppercase font-bold tracking-wider">Downloading</span>`;
      } else if (task.status === 'paused') {
        statusBadge = `<span class="bg-warning/10 text-warning border border-warning/30 px-1.5 py-0.5 rounded text-[10px] uppercase font-bold tracking-wider">Paused</span>`;
      } else if (task.status === 'failed') {
        statusBadge = `<span class="bg-error/10 text-error border border-error/30 px-1.5 py-0.5 rounded text-[10px] uppercase font-bold tracking-wider">Failed</span>`;
      } else if (task.status === 'finished') {
        statusBadge = `<span class="bg-success/10 text-success border border-success/30 px-1.5 py-0.5 rounded text-[10px] uppercase font-bold tracking-wider">Completed</span>`;
      } else {
        statusBadge = `<span class="bg-black-400 text-black-100 px-2 py-0.5 rounded text-[10px] uppercase font-bold tracking-wider">${task.status}</span>`;
      }

      let progressInfo = '';
      if (task.status === 'running') {
        progressInfo = `
          <div class="flex items-center space-x-2 min-w-[120px]">
            <div class="w-24 bg-black-500 rounded-full h-1.5 overflow-hidden">
              <div class="bg-accent h-1.5 rounded-full" style="width: ${progressPct}%"></div>
            </div>
            <span class="text-[10px] font-mono">${progressPct}% (${downloadedMb}/${totalMb} MB)</span>
          </div>
        `;
      } else if (task.status === 'queued') {
        progressInfo = `<span class="text-[10px] text-black-100">Waiting in queue</span>`;
      } else if (task.status === 'paused') {
        progressInfo = `
          <div class="flex items-center space-x-2 min-w-[120px]">
            <div class="w-24 bg-black-500 rounded-full h-1.5 overflow-hidden opacity-50">
              <div class="bg-warning h-1.5 rounded-full" style="width: ${progressPct}%"></div>
            </div>
            <span class="text-[10px] font-mono text-black-100">${progressPct}% (${downloadedMb}/${totalMb} MB)</span>
          </div>
        `;
      } else if (task.status === 'failed' && task.error) {
        progressInfo = `<span class="text-[10px] text-error truncate max-w-[200px]" title="${escapeHtml(task.error)}">${escapeHtml(task.error)}</span>`;
      } else {
        progressInfo = `<span class="text-[10px] text-black-100">-</span>`;
      }

      const showPause = task.status === 'running';
      const showResume = task.status === 'paused' || task.status === 'failed';
      const showCancel = task.status === 'running' || task.status === 'queued' || task.status === 'paused';

      return `
        <tr class="hover:bg-black-500/30 text-xs text-white">
          <td class="px-4 py-3 font-semibold text-white max-w-[150px] truncate" title="${escapeHtml(task.podcastTitle || task.name || task.type || 'Task')}">
            ${escapeHtml(task.podcastTitle || task.name || task.type || 'Task')}
          </td>
          <td class="px-4 py-3 max-w-[250px] truncate" title="${escapeHtml(task.episodeTitle || task.description || '')}">
            ${escapeHtml(task.episodeTitle || task.description || '')}
          </td>
          <td class="px-4 py-3">${statusBadge}</td>
          <td class="px-4 py-3">${progressInfo}</td>
          <td class="px-4 py-3 text-right">
            <div class="flex justify-end space-x-1.5">
              ${showPause ? `
                <button class="task-action-btn hover:text-white text-black-100 p-1 rounded hover:bg-black-400" data-id="${task.id}" data-action="pause" title="Pause Download">
                  <span class="material-symbols text-sm">pause</span>
                </button>
              ` : ''}
              ${showResume ? `
                <button class="task-action-btn hover:text-accent text-black-100 p-1 rounded hover:bg-black-400" data-id="${task.id}" data-action="resume" title="Resume/Retry Download">
                  <span class="material-symbols text-sm">play_arrow</span>
                </button>
              ` : ''}
              ${showCancel ? `
                <button class="task-action-btn hover:text-error text-black-100 p-1 rounded hover:bg-black-400" data-id="${task.id}" data-action="cancel" title="Cancel Download">
                  <span class="material-symbols text-sm">cancel</span>
                </button>
              ` : ''}
            </div>
          </td>
        </tr>
      `;
    }).join('');

    tbody.querySelectorAll('.task-action-btn').forEach(btn => {
      btn.onclick = async (e) => {
        e.preventDefault();
        const taskId = btn.dataset.id;
        const action = btn.dataset.action;
        try {
          await request('POST', `/api/tasks/${taskId}/${action}`);
          showToast(`Task ${action}d successfully`, 'success');
          updateTasksList();
        } catch (err) {
          showToast(`Failed to ${action} task: ` + err.message, 'error');
        }
      };
    });

  } catch (err) {
    console.error('Failed to update tasks list:', err);
  }
}
