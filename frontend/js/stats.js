// js/stats.js
import { request } from './api.js';

export async function loadStats() {
  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Listening Stats';
  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = '';

  let activeTab = 'my';
  let user = null;
  try {
    user = await request('GET', '/api/me');
  } catch (e) {
    console.error('Failed to get current user details', e);
  }
  const isAdmin = user && (user.type === 'root' || user.type === 'admin');

  // Format duration helper
  const formatDuration = (seconds) => {
    if (!seconds || seconds <= 0) return '0m';
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (hours > 0) {
      return `${hours}h ${minutes}m`;
    }
    if (minutes > 0) {
      return `${minutes}m`;
    }
    return `${Math.round(seconds)}s`;
  };

  const fetchAndRender = async () => {
    container.innerHTML = `<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent mx-auto mt-20"></div>`;
    try {
      const url = activeTab === 'server' ? '/api/server-listening-stats' : '/api/me/listening-stats';
      const stats = await request('GET', url);
      renderUI(stats);
    } catch (err) {
      container.innerHTML = `
        <div class="text-center py-20">
          <span class="material-symbols text-5xl text-error mb-2">error</span>
          <p class="text-white text-lg">Failed to load statistics</p>
          <p class="text-black-100 text-sm mt-1">${err.message}</p>
          <button id="retry-stats-btn" class="mt-6 bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-sm">Retry</button>
        </div>
      `;
      const retryBtn = document.getElementById('retry-stats-btn');
      if (retryBtn) retryBtn.onclick = fetchAndRender;
    }
  };

  const renderUI = (stats) => {
    const totalTimeStr = formatDuration(stats.totalTime);
    const todayTimeStr = formatDuration(stats.today);
    
    const itemsList = Object.entries(stats.items || {}).map(([id, item]) => ({ id, ...item }));
    const uniqueItemsCount = itemsList.length;

    // Calculate maximum time for normalization
    const maxDayOfWeek = Math.max(...Object.values(stats.dayOfWeek || {}), 1);
    
    // Day names mapping
    const dayNames = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

    // Group stats.days by Month
    const monthsMap = {};
    Object.entries(stats.days || {}).forEach(([dateStr, seconds]) => {
      const monthStr = dateStr.substring(0, 7); // YYYY-MM
      monthsMap[monthStr] = (monthsMap[monthStr] || 0) + seconds;
    });
    const monthsList = Object.entries(monthsMap).sort((a, b) => a[0].localeCompare(b[0])).slice(-6); // last 6 months
    const maxMonthVal = Math.max(...monthsList.map(m => m[1]), 1);

    // Top authors sorted
    const topAuthorsList = Object.entries(stats.topAuthors || {})
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5);

    // Top genres sorted
    const topGenresList = Object.entries(stats.topGenres || {})
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5);

    // Top users sorted (server wide)
    const topUsersList = Object.entries(stats.topUsers || {})
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5);

    let tabsHtml = '';
    if (isAdmin) {
      tabsHtml = `
        <div class="flex space-x-4 border-b border-black-400 pb-2 mb-6">
          <button id="tab-my-stats" class="px-4 py-2 text-sm font-semibold rounded-md transition-colors ${activeTab === 'my' ? 'bg-accent text-primary font-bold' : 'text-white hover:bg-black-500'}" data-tab="my">My Stats</button>
          <button id="tab-server-stats" class="px-4 py-2 text-sm font-semibold rounded-md transition-colors ${activeTab === 'server' ? 'bg-accent text-primary font-bold' : 'text-white hover:bg-black-500'}" data-tab="server">Server Stats</button>
        </div>
      `;
    }

    container.innerHTML = `
      <div class="p-6 max-w-6xl mx-auto space-y-8 text-white">
        ${tabsHtml}

        <!-- 1. Header Overview Cards -->
        <div class="grid grid-cols-1 md:grid-cols-4 gap-6">
          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">schedule</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Total Time</p>
              <h3 class="text-2xl font-bold text-white mt-1">${totalTimeStr}</h3>
            </div>
          </div>

          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">today</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Today</p>
              <h3 class="text-2xl font-bold text-white mt-1">${todayTimeStr}</h3>
            </div>
          </div>

          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">headphones</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Unique Items</p>
              <h3 class="text-2xl font-bold text-white mt-1">${uniqueItemsCount}</h3>
            </div>
          </div>

          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">history</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Recent Sessions</p>
              <h3 class="text-2xl font-bold text-white mt-1">${stats.recentSessions ? stats.recentSessions.length : 0}</h3>
            </div>
          </div>
        </div>

        <!-- 2. Charts section -->
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-8">
          <!-- Day of Week Chart -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md flex flex-col justify-between">
            <div>
              <h3 class="text-lg font-semibold mb-6 flex items-center space-x-2">
                <span class="material-symbols text-accent text-xl">bar_chart</span>
                <span>Listening by Day of Week</span>
              </h3>
              <div class="flex items-end justify-between h-48 pt-4 px-2">
                ${[0, 1, 2, 3, 4, 5, 6].map(dayIndex => {
                  const val = stats.dayOfWeek ? (stats.dayOfWeek[String(dayIndex)] || 0) : 0;
                  const pct = Math.max((val / maxDayOfWeek) * 100, 3); // minimum 3% for visibility
                  const formatted = formatDuration(val);
                  return `
                    <div class="flex flex-col items-center flex-1 group">
                      <div class="relative w-full flex justify-center">
                        <!-- Tooltip -->
                        <span class="absolute bottom-full mb-2 bg-black-500 text-xs px-2 py-1 rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap pointer-events-none z-10">
                          ${formatted}
                        </span>
                        <!-- Bar -->
                        <div class="w-6 sm:w-8 bg-accent rounded-t transition-all duration-500 hover:bg-opacity-80" style="height: ${pct}px; max-height: 120px;"></div>
                      </div>
                      <span class="text-xs text-black-100 mt-2 font-mono">${dayNames[dayIndex].substring(0, 3)}</span>
                    </div>
                  `;
                }).join('')}
              </div>
            </div>
          </div>

          <!-- Monthly Listening Chart -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md flex flex-col justify-between">
            <div>
              <h3 class="text-lg font-semibold mb-6 flex items-center space-x-2">
                <span class="material-symbols text-accent text-xl">insights</span>
                <span>Monthly Listening Trend</span>
              </h3>
              <div class="flex items-end justify-between h-48 pt-4 px-2">
                ${monthsList.length === 0 ? `
                  <div class="w-full text-center text-black-100 py-10">No monthly trends.</div>
                ` : monthsList.map(([monthStr, val]) => {
                  const pct = Math.max((val / maxMonthVal) * 100, 3);
                  const formatted = formatDuration(val);
                  const date = new Date(monthStr + '-02');
                  const formattedMonth = isNaN(date.getTime()) ? monthStr : date.toLocaleDateString(undefined, { month: 'short', year: '2-digit' });
                  return `
                    <div class="flex flex-col items-center flex-1 group">
                      <div class="relative w-full flex justify-center">
                        <!-- Tooltip -->
                        <span class="absolute bottom-full mb-2 bg-black-500 text-xs px-2 py-1 rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap pointer-events-none z-10">
                          ${formatted}
                        </span>
                        <!-- Bar -->
                        <div class="w-6 sm:w-8 bg-accent rounded-t transition-all duration-500 hover:bg-opacity-80" style="height: ${pct}px; max-height: 120px;"></div>
                      </div>
                      <span class="text-xs text-black-100 mt-2 font-mono">${formattedMonth}</span>
                    </div>
                  `;
                }).join('')}
              </div>
            </div>
          </div>
        </div>

        <!-- 3. Top Stats Lists section -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-8">
          <!-- Most Listened Items -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
            <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
              <span class="material-symbols text-accent text-xl">grade</span>
              <span>Most Listened Items</span>
            </h3>
            <div class="space-y-4 max-h-[250px] overflow-y-auto pr-2 no-scroll">
              ${itemsList.length === 0 ? `
                <div class="text-center text-black-100 py-10">No items in history.</div>
              ` : itemsList.sort((a, b) => b.timeListened - a.timeListened).slice(0, 5).map((item) => {
                const totalSec = stats.totalTime || 1;
                const progressPct = Math.round((item.timeListened / totalSec) * 100);
                return `
                  <div class="space-y-1">
                    <div class="flex justify-between items-center text-sm">
                      <div class="truncate max-w-[70%] cursor-pointer group" onclick="window.navigateTo('/item/${item.id}')">
                        <span class="font-medium text-white group-hover:text-accent transition-colors">${item.title}</span>
                        ${item.author ? `<span class="text-xs text-black-100 block truncate group-hover:text-accent/80 transition-colors">by ${item.author}</span>` : ''}
                      </div>
                      <span class="text-xs font-mono text-accent">${formatDuration(item.timeListened)}</span>
                    </div>
                    <div class="w-full bg-black-500 h-1.5 rounded-full overflow-hidden">
                      <div class="bg-accent h-full rounded-full" style="width: ${progressPct}%"></div>
                    </div>
                  </div>
                `;
              }).join('')}
            </div>
          </div>

          <!-- Top Authors -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
            <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
              <span class="material-symbols text-accent text-xl">person</span>
              <span>Top Authors</span>
            </h3>
            <div class="space-y-4 max-h-[250px] overflow-y-auto pr-2 no-scroll">
              ${topAuthorsList.length === 0 ? `
                <div class="text-center text-black-100 py-10">No author stats.</div>
              ` : topAuthorsList.map(([author, seconds]) => {
                const totalSec = stats.totalTime || 1;
                const progressPct = Math.round((seconds / totalSec) * 100);
                return `
                  <div class="space-y-1">
                    <div class="flex justify-between items-center text-sm">
                      <span class="font-medium text-white truncate max-w-[70%]">${author}</span>
                      <span class="text-xs font-mono text-accent">${formatDuration(seconds)}</span>
                    </div>
                    <div class="w-full bg-black-500 h-1.5 rounded-full overflow-hidden">
                      <div class="bg-accent h-full rounded-full" style="width: ${progressPct}%"></div>
                    </div>
                  </div>
                `;
              }).join('')}
            </div>
          </div>

          <!-- Top Genres / Top Users -->
          ${activeTab === 'server' ? `
            <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
              <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
                <span class="material-symbols text-accent text-xl">group</span>
                <span>Top Users</span>
              </h3>
              <div class="space-y-4 max-h-[250px] overflow-y-auto pr-2 no-scroll">
                ${topUsersList.length === 0 ? `
                  <div class="text-center text-black-100 py-10">No user stats.</div>
                ` : topUsersList.map(([username, seconds]) => {
                  const totalSec = stats.totalTime || 1;
                  const progressPct = Math.round((seconds / totalSec) * 100);
                  return `
                    <div class="space-y-1">
                      <div class="flex justify-between items-center text-sm">
                        <span class="font-medium text-white truncate max-w-[70%]">${username}</span>
                        <span class="text-xs font-mono text-accent">${formatDuration(seconds)}</span>
                      </div>
                      <div class="w-full bg-black-500 h-1.5 rounded-full overflow-hidden">
                        <div class="bg-accent h-full rounded-full" style="width: ${progressPct}%"></div>
                      </div>
                    </div>
                  `;
                }).join('')}
              </div>
            </div>
          ` : `
            <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
              <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
                <span class="material-symbols text-accent text-xl">sell</span>
                <span>Top Genres</span>
              </h3>
              <div class="space-y-4 max-h-[250px] overflow-y-auto pr-2 no-scroll">
                ${topGenresList.length === 0 ? `
                  <div class="text-center text-black-100 py-10">No genre stats.</div>
                ` : topGenresList.map(([genre, seconds]) => {
                  const totalSec = stats.totalTime || 1;
                  const progressPct = Math.round((seconds / totalSec) * 100);
                  return `
                    <div class="space-y-1">
                      <div class="flex justify-between items-center text-sm">
                        <span class="font-medium text-white truncate max-w-[70%]">${genre}</span>
                        <span class="text-xs font-mono text-accent">${formatDuration(seconds)}</span>
                      </div>
                      <div class="w-full bg-black-500 h-1.5 rounded-full overflow-hidden">
                        <div class="bg-accent h-full rounded-full" style="width: ${progressPct}%"></div>
                      </div>
                    </div>
                  `;
                }).join('')}
              </div>
            </div>
          `}
        </div>

        <!-- 4. Recent Sessions List with pagination -->
        <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
          <div class="flex justify-between items-center mb-6">
            <h3 class="text-lg font-semibold flex items-center space-x-2">
              <span class="material-symbols text-accent text-xl">history</span>
              <span>All Playback Sessions</span>
            </h3>
          </div>
          
          <div class="overflow-x-auto">
            <table class="w-full text-left border-collapse text-sm">
              <thead>
                <tr class="border-b border-black-400 text-black-100 font-medium">
                  <th class="pb-3 pr-4">Item</th>
                  ${activeTab === 'server' ? '<th class="pb-3 pr-4">User</th>' : ''}
                  <th class="pb-3 pr-4">Date</th>
                  <th class="pb-3 pr-4">Device</th>
                  <th class="pb-3 pr-4">Play Method</th>
                  <th class="pb-3 text-right">Time Listened</th>
                </tr>
              </thead>
              <tbody id="sessions-table-body">
                <!-- Rendered dynamically -->
              </tbody>
            </table>
          </div>

          <!-- Pagination Controls -->
          <div class="flex justify-between items-center mt-6 pt-4 border-t border-black-400 text-xs text-black-50">
            <span id="sessions-page-info">Showing page 1 of 1</span>
            <div class="flex space-x-2">
              <button id="sessions-prev-btn" class="bg-black-500 hover:bg-black-400 text-white font-semibold px-3 py-1.5 rounded disabled:opacity-50">Previous</button>
              <button id="sessions-next-btn" class="bg-black-500 hover:bg-black-400 text-white font-semibold px-3 py-1.5 rounded disabled:opacity-50">Next</button>
            </div>
          </div>
        </div>
      </div>
    `;

    // Hook tabs events
    if (isAdmin) {
      document.getElementById('tab-my-stats').onclick = () => {
        activeTab = 'my';
        fetchAndRender();
      };
      document.getElementById('tab-server-stats').onclick = () => {
        activeTab = 'server';
        fetchAndRender();
      };
    }

    // Paginate functions
    let currentPage = 0;
    const itemsPerPage = 10;

    const renderSessionsTable = async (page) => {
      const tableBody = document.getElementById('sessions-table-body');
      if (!tableBody) return;

      tableBody.innerHTML = `<tr><td colspan="${activeTab === 'server' ? 6 : 5}" class="py-10 text-center"><div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div></td></tr>`;

      try {
        const sessionsUrl = activeTab === 'server' ? `/api/server-listening-sessions?page=${page}&itemsPerPage=${itemsPerPage}` : `/api/me/listening-sessions?page=${page}&itemsPerPage=${itemsPerPage}`;
        const paginated = await request('GET', sessionsUrl);
        const sessions = paginated.sessions || [];
        const total = paginated.total || 0;
        const totalPages = Math.max(Math.ceil(total / itemsPerPage), 1);

        document.getElementById('sessions-page-info').textContent = `Showing page ${page + 1} of ${totalPages} (${total} total sessions)`;
        document.getElementById('sessions-prev-btn').disabled = (page === 0);
        document.getElementById('sessions-next-btn').disabled = (page >= totalPages - 1);

        if (sessions.length === 0) {
          tableBody.innerHTML = `<tr><td colspan="${activeTab === 'server' ? 6 : 5}" class="py-10 text-center text-black-100">No sessions recorded yet. Start listening to generate stats!</td></tr>`;
          return;
        }

        tableBody.innerHTML = sessions.map(sess => {
          let dateStr = 'Unknown';
          if (sess.updatedAt) {
            const dateObj = new Date(sess.updatedAt.replace(' ', 'T'));
            if (!isNaN(dateObj)) {
              dateStr = dateObj.toLocaleDateString(undefined, {
                year: 'numeric', month: 'short', day: 'numeric',
                hour: '2-digit', minute: '2-digit'
              });
            } else {
              dateStr = sess.updatedAt;
            }
          }
          return `
            <tr class="border-b border-black-500 hover:bg-black-500/20">
              <td class="py-3 pr-4 font-medium text-white max-w-[200px] truncate cursor-pointer group" onclick="window.navigateTo('/item/${sess.mediaItemId}')">
                <span class="group-hover:text-accent transition-colors">${sess.title || 'Unknown Item'}</span>
                ${sess.author ? `<span class="text-xs text-black-100 block truncate group-hover:text-accent/80 transition-colors">by ${sess.author}</span>` : ''}
              </td>
              ${activeTab === 'server' ? `<td class="py-3 pr-4 text-black-50">${sess.username || 'Unknown User'}</td>` : ''}
              <td class="py-3 pr-4 text-black-50">${dateStr}</td>
              <td class="py-3 pr-4 text-black-50">${sess.deviceInfo || 'Web Client'}</td>
              <td class="py-3 pr-4"><span class="bg-black-500 px-2 py-0.5 rounded text-xs border border-black-400 font-mono">${sess.playMethod || 'HLS'}</span></td>
              <td class="py-3 text-right font-mono font-medium text-accent">${formatDuration(sess.timeListened)}</td>
            </tr>
          `;
        }).join('');
      } catch (err) {
        tableBody.innerHTML = `<tr><td colspan="${activeTab === 'server' ? 6 : 5}" class="py-10 text-center text-error">Failed to load playback sessions.</td></tr>`;
      }
    };

    // Setup paginator listeners
    document.getElementById('sessions-prev-btn').onclick = () => {
      if (currentPage > 0) {
        currentPage--;
        renderSessionsTable(currentPage);
      }
    };
    document.getElementById('sessions-next-btn').onclick = () => {
      currentPage++;
      renderSessionsTable(currentPage);
    };

    // Load initial table
    renderSessionsTable(0);
  };

  // Initial load
  await fetchAndRender();
}
