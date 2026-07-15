// js/stats.js
import { request } from './api.js';
import { getActiveLibraryId, getLibrariesList, getActiveLibrary } from './library.js';

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

  // Format duration helper (short format)
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

  // Format duration helper (long format)
  const formatDurationLong = (seconds) => {
    if (!seconds || seconds <= 0) return '0 hours';
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    
    const parts = [];
    if (days > 0) parts.push(`${days} day${days === 1 ? '' : 's'}`);
    if (hours > 0) parts.push(`${hours} hour${hours === 1 ? '' : 's'}`);
    if (minutes > 0 && days === 0) parts.push(`${minutes} minute${minutes === 1 ? '' : 's'}`);
    return parts.join(', ');
  };

  // Format bytes helper
  const formatBytes = (bytes, decimals = 2) => {
    if (!bytes || bytes === 0) return '0 Bytes';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
  };

  const fetchAndRender = async () => {
    container.innerHTML = `<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent mx-auto mt-20"></div>`;
    try {
      let stats = null;
      let libraryId = getActiveLibraryId();
      if (activeTab === 'library') {
        if (!libraryId) {
          const libs = getLibrariesList();
          if (libs && libs.length > 0) libraryId = libs[0].id;
        }
        if (libraryId) {
          stats = await request('GET', `/api/libraries/${libraryId}/stats`);
        } else {
          stats = { totalItems: 0, totalDuration: 0, numAudioFiles: 0, totalSize: 0, totalAuthors: 0 };
        }
      } else {
        const url = activeTab === 'server' ? '/api/server-listening-stats' : '/api/me/listening-stats';
        stats = await request('GET', url);
      }
      renderUI(stats, libraryId);
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

  const renderUI = (stats, libraryId) => {
    let postRenderHook = null;
    let tabsHtml = `
      <div class="flex space-x-4 border-b border-black-400 pb-2 mb-6">
        <button id="tab-my-stats" class="px-4 py-2 text-sm font-semibold rounded-md transition-colors ${activeTab === 'my' ? 'bg-accent text-primary font-bold' : 'text-white hover:bg-black-500'}" data-tab="my">My Stats</button>
        <button id="tab-library-stats" class="px-4 py-2 text-sm font-semibold rounded-md transition-colors ${activeTab === 'library' ? 'bg-accent text-primary font-bold' : 'text-white hover:bg-black-500'}" data-tab="library">Library Stats</button>
    `;
    if (isAdmin) {
      tabsHtml += `
        <button id="tab-server-stats" class="px-4 py-2 text-sm font-semibold rounded-md transition-colors ${activeTab === 'server' ? 'bg-accent text-primary font-bold' : 'text-white hover:bg-black-500'}" data-tab="server">Server Stats</button>
      `;
    }
    tabsHtml += `</div>`;

    let viewContentHtml = '';

    if (activeTab === 'my') {
      const totalTimeStr = formatDuration(stats.totalTime);
      const todayTimeStr = formatDuration(stats.today);
      const itemsList = Object.entries(stats.items || {}).map(([id, item]) => ({ id, ...item }));
      const uniqueItemsCount = itemsList.length;

      // Calculate last 7 days of listening (for the line chart)
      const last7DaysOfListening = [];
      const today = new Date();
      for (let i = 6; i >= 0; i--) {
        const d = new Date();
        d.setDate(today.getDate() - i);
        const dateStr = d.toISOString().split('T')[0];
        const seconds = stats.days ? (stats.days[dateStr] || 0) : 0;
        const minutes = Math.round(seconds / 60);
        const dayLabel = d.toLocaleDateString(undefined, { weekday: 'short' });
        last7DaysOfListening.push({ dateStr, minutesListening: minutes, label: dayLabel });
      }

      const mostListenedDay = Math.max(...last7DaysOfListening.map(d => d.minutesListening), 0);
      const factor = Math.ceil(mostListenedDay / 5);
      const yAxisFactor = factor > 25 ? Math.ceil(factor / 5) * 5 : Math.max(1, factor);
      const yAxisLabels = [];
      for (let i = 6; i >= 0; i--) {
        yAxisLabels.push(i * yAxisFactor);
      }

      // Points for SVG path
      const chartWidth = 384;
      const chartHeight = 268;
      const chartContentWidth = 330;
      const chartContentMarginLeft = 34;
      const chartContentMarginBottom = 20;
      const daySpacing = chartContentWidth / 6;

      const points = [];
      for (let i = 0; i < 7; i++) {
        const minutes = last7DaysOfListening[i].minutesListening || 0;
        const yPercent = minutes / ((yAxisFactor * 6) || 1);
        points.push({
          x: chartContentMarginLeft + daySpacing * i,
          y: (288 - chartContentMarginBottom) - (200 * yPercent)
        });
      }

      // SVG path string
      let pathD = '';
      if (points.length > 0) {
        pathD = `M ${points[0].x} ${points[0].y} ` + points.slice(1).map(p => `L ${p.x} ${p.y}`).join(' ');
      }

      // Total minutes listened this week
      const totalMinutesListeningThisWeek = last7DaysOfListening.reduce((acc, d) => acc + d.minutesListening, 0);
      const averageMinutesPerDay = Math.round(totalMinutesListeningThisWeek / 7);

      // Days in a row streak calculation
      let daysInARow = 0;
      const daysMap = stats.days || {};
      while (true) {
        const d = new Date();
        d.setDate(today.getDate() - daysInARow - 1);
        const datestr = d.toISOString().split('T')[0];
        if (!daysMap[datestr] || daysMap[datestr] === 0) {
          const todayStr = today.toISOString().split('T')[0];
          if (daysMap[todayStr]) {
            daysInARow++;
          }
          break;
        }
        daysInARow++;
        if (daysInARow > 9999) break;
      }

      // Heatmap calendar variables
      const numDaysInTheLastYear = 365;
      const dayOfWeekToday = today.getDay();
      const weeksToShow = 52;
      const daysToShow = weeksToShow * 7 + dayOfWeekToday;
      const firstDay = new Date();
      firstDay.setDate(today.getDate() - numDaysInTheLastYear);

      const dates = [];
      let daysListenedInTheLastYear = 0;
      let heatMax = 0;
      let heatMin = Infinity;

      for (let i = 0; i <= numDaysInTheLastYear; i++) {
        const dateObj = new Date(firstDay);
        dateObj.setDate(firstDay.getDate() + i);
        const dateString = dateObj.toISOString().split('T')[0];

        if (daysMap[dateString] > 0) {
          daysListenedInTheLastYear++;
        }

        const visibleDayIndex = i - (numDaysInTheLastYear - daysToShow);
        if (visibleDayIndex < 0) {
          continue;
        }

        const value = daysMap[dateString] || 0;
        const datePretty = dateObj.toLocaleDateString(undefined, {
          month: 'short', day: 'numeric', year: 'numeric'
        });
        const monthString = dateObj.toLocaleDateString(undefined, { month: 'short' });
        const dayOfMonth = dateObj.getDate();

        const item = {
          col: Math.floor(visibleDayIndex / 7),
          row: visibleDayIndex % 7,
          dateString,
          datePretty,
          monthString,
          dayOfMonth,
          value
        };
        dates.push(item);

        if (value > 0) {
          if (value > heatMax) heatMax = value;
          if (value < heatMin) heatMin = value;
        }
      }

      const heatRange = heatMax - heatMin + 0.01;
      const bgColors = ['rgb(45,45,45)', 'rgb(14, 68, 41)', 'rgb(0, 109, 50)', 'rgb(38, 166, 65)', 'rgb(57, 211, 83)'];
      const outlineColors = ['rgba(255,255,255,0.03)', 'rgba(255,255,255,0.03)'];

      const blocksHtml = dates.map(block => {
        let bgColor = bgColors[0];
        let outlineColor = outlineColors[0];
        if (block.value > 0) {
          const percentOfAvg = (block.value - heatMin) / heatRange;
          const bgIndex = Math.floor(percentOfAvg * 4) + 1;
          bgColor = bgColors[bgIndex] || bgColors[4];
        }

        const mins = Math.round(block.value / 60);

        return `
          <div class="group absolute h-2.5 w-2.5 rounded-xs cursor-pointer hover:scale-125 transition-transform duration-100 z-10" style="transform:translate(${block.col * 13}px,${block.row * 13}px); background-color:${bgColor}; outline:1px solid ${outlineColor}; outline-offset:-1px;">
            <div class="pointer-events-none absolute bottom-full left-1/2 -translate-x-1/2 mb-1.5 z-50 opacity-0 group-hover:opacity-100 bg-black-700 text-white text-[10px] px-2 py-1 rounded shadow whitespace-nowrap transition-opacity duration-150">
              ${mins} min${mins === 1 ? '' : 's'} listening on ${block.datePretty}
            </div>
          </div>
        `;
      }).join('');

      const monthLabels = [];
      let lastMonth = null;
      for (let i = 0; i < dates.length; i++) {
        if (dates[i].monthString !== lastMonth) {
          const weekOfMonth = Math.floor(dates[i].dayOfMonth / 7);
          if (weekOfMonth <= 2) {
            monthLabels.push({
              label: dates[i].monthString,
              col: dates[i].col
            });
            lastMonth = dates[i].monthString;
          }
        }
      }

      const monthLabelsHtml = monthLabels.map(ml => `
        <div style="transform: translate(${ml.col * 13}px, -15px); line-height: 10px; font-size: 10px; position: absolute;" class="text-black-300 font-semibold">${ml.label}</div>
      `).join('');

      viewContentHtml = `
        <!-- KPI Cards -->
        <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-5 gap-6">
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
              <span class="material-symbols text-3xl">calendar_today</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Days Listened</p>
              <h3 class="text-2xl font-bold text-white mt-1">${stats.daysListened || 0}</h3>
            </div>
          </div>

          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">check_circle</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Items Finished</p>
              <h3 class="text-2xl font-bold text-white mt-1">${stats.itemsFinished || 0}</h3>
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
        </div>

        <!-- Charts side by side -->
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-8">
          <!-- 7-day Line Chart -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md flex flex-col items-center">
            <h1 class="text-xl mb-4 font-semibold w-full text-left">Minutes Listening</h1>
            <div class="relative w-[384px] h-[288px] my-4">
              <!-- Y Axis Grid and Labels -->
              ${yAxisLabels.map((lbl, idx) => {
                const y = (288 - chartContentMarginBottom) - (200 * (idx / 6));
                return `
                  <div class="absolute right-[360px] text-xs font-semibold text-black-100 pr-2" style="top: ${y - 8}px;">${lbl}</div>
                  <div class="absolute left-[34px] right-0 h-px bg-white/10" style="top: ${y}px; width: 330px;"></div>
                `;
              }).join('')}

              <!-- The SVGs -->
              <svg width="384" height="288" class="absolute inset-0 overflow-visible pointer-events-none">
                <!-- SVG path line -->
                <path d="${pathD}" fill="none" stroke="#eab308" stroke-width="2"></path>
              </svg>

              <!-- Points and Tooltips -->
              ${points.map((p, idx) => `
                <div class="absolute group cursor-pointer z-20" style="left: ${p.x - 4}px; top: ${p.y - 4}px; width: 8px; height: 8px;">
                  <div class="h-2 w-2 bg-yellow-400 hover:bg-yellow-300 rounded-full transform duration-150 transition-transform hover:scale-125"></div>
                  <!-- Tooltip -->
                  <div class="pointer-events-none absolute bottom-full left-1/2 -translate-x-1/2 mb-2 bg-black-700 text-white text-xs px-2 py-1 rounded shadow whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity">
                    ${last7DaysOfListening[idx].minutesListening} mins
                  </div>
                </div>
              `).join('')}

              <!-- Day Labels -->
              ${last7DaysOfListening.map((d, idx) => {
                const x = chartContentMarginLeft + daySpacing * idx;
                return `
                  <div class="absolute text-xs text-black-100 text-center font-semibold" style="left: ${x - 20}px; width: 40px; bottom: 4px;">${d.label}</div>
                `;
              }).join('')}
            </div>

            <!-- Chart summary row -->
            <div class="flex justify-between w-full pt-10 border-t border-black-400 mt-6">
              <div class="text-center flex-1">
                <p class="text-xs text-black-100 font-semibold">This Week</p>
                <p class="text-3xl font-bold text-accent">${totalMinutesListeningThisWeek}</p>
                <p class="text-xs text-black-100">Minutes</p>
              </div>
              <div class="text-center flex-1">
                <p class="text-xs text-black-100 font-semibold">Daily Average</p>
                <p class="text-3xl font-bold text-accent">${averageMinutesPerDay}</p>
                <p class="text-xs text-black-100">Minutes</p>
              </div>
              <div class="text-center flex-1">
                <p class="text-xs text-black-100 font-semibold">Best Day</p>
                <p class="text-3xl font-bold text-accent">${mostListenedDay}</p>
                <p class="text-xs text-black-100">Minutes</p>
              </div>
              <div class="text-center flex-1">
                <p class="text-xs text-black-100 font-semibold">Streak</p>
                <p class="text-3xl font-bold text-accent">${daysInARow}</p>
                <p class="text-xs text-black-100">Days in a row</p>
              </div>
            </div>
          </div>

          <!-- Recent sessions -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md flex flex-col justify-between">
            <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
              <span class="material-symbols text-accent text-xl">history</span>
              <span>Recent Playback Sessions</span>
            </h3>
            <div class="space-y-4 max-h-[340px] overflow-y-auto pr-2 no-scroll flex-1">
              <div id="recent-sessions-list" class="space-y-3">
                <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto mt-10"></div>
              </div>
            </div>
          </div>
        </div>

        <!-- Heatmap Calendar -->
        <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md w-full overflow-x-auto">
          <div id="heatmap" class="min-w-[750px]">
            <div class="mx-auto select-none" style="height: 190px; width: 741px; position: relative;">
              <p class="mb-4 px-1 text-sm text-black-100 font-semibold">${daysListenedInTheLastYear} days listened in the last year</p>
              <div class="border border-white/10 rounded-sm py-4 w-full" style="background-color: #1a1a1a; height: 140px; position: relative;">
                <div style="width: 689px; height: 91px;" class="ml-12 mt-4 absolute">
                  <!-- Day Labels -->
                  <div style="transform: translate(-25px, 13px); line-height: 10px; font-size: 10px; position: absolute;" class="text-black-100 font-semibold">Mon</div>
                  <div style="transform: translate(-25px, 39px); line-height: 10px; font-size: 10px; position: absolute;" class="text-black-100 font-semibold">Wed</div>
                  <div style="transform: translate(-25px, 65px); line-height: 10px; font-size: 10px; position: absolute;" class="text-black-100 font-semibold">Fri</div>
                  
                  <!-- Month Labels -->
                  ${monthLabelsHtml}
                  
                  <!-- Blocks -->
                  ${blocksHtml}
                  
                  <!-- Legend -->
                  <div class="flex py-2 px-4 absolute w-full left-0 items-center font-semibold" style="margin-top: 91px;">
                    <div class="grow"></div>
                    <p style="font-size: 10px; line-height: 10px" class="text-black-100 px-1">Less</p>
                    <div class="h-2.5 w-2.5 rounded-xs" style="margin-left: 1.5px; margin-right: 1.5px; background-color: rgb(45,45,45); outline: 1px solid rgba(255,255,255,0.03); outline-offset: -1px;"></div>
                    <div class="h-2.5 w-2.5 rounded-xs" style="margin-left: 1.5px; margin-right: 1.5px; background-color: rgb(14,68,41); outline: 1px solid rgba(255,255,255,0.03); outline-offset: -1px;"></div>
                    <div class="h-2.5 w-2.5 rounded-xs" style="margin-left: 1.5px; margin-right: 1.5px; background-color: rgb(0,109,50); outline: 1px solid rgba(255,255,255,0.03); outline-offset: -1px;"></div>
                    <div class="h-2.5 w-2.5 rounded-xs" style="margin-left: 1.5px; margin-right: 1.5px; background-color: rgb(38,166,65); outline: 1px solid rgba(255,255,255,0.03); outline-offset: -1px;"></div>
                    <div class="h-2.5 w-2.5 rounded-xs" style="margin-left: 1.5px; margin-right: 1.5px; background-color: rgb(57,211,83); outline: 1px solid rgba(255,255,255,0.03); outline-offset: -1px;"></div>
                    <p style="font-size: 10px; line-height: 10px" class="text-black-100 px-1">More</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      `;

      // Trigger fetch of recent sessions list after render
      postRenderHook = () => {
        const listContainer = document.getElementById('recent-sessions-list');
        if (!listContainer) return;
        request('GET', '/api/me/listening-sessions?page=0&itemsPerPage=10')
          .then(paginated => {
            const sessions = paginated.sessions || [];
            if (sessions.length === 0) {
              listContainer.innerHTML = `<p class="text-black-100 text-center py-10 text-sm font-semibold">No recent sessions found.</p>`;
              return;
            }
            listContainer.innerHTML = sessions.map(sess => {
              let dateStr = 'Unknown';
              if (sess.updatedAt) {
                const dateObj = new Date(sess.updatedAt.replace(' ', 'T'));
                if (!isNaN(dateObj)) {
                  dateStr = dateObj.toLocaleDateString(undefined, {
                    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
                  });
                }
              }
              return `
                <div class="flex justify-between items-center border-b border-black-500 pb-2 text-sm">
                  <div class="truncate max-w-[70%] cursor-pointer group" onclick="window.navigateTo('/item/${sess.mediaItemId}')">
                    <span class="font-medium text-white group-hover:text-accent transition-colors block truncate font-semibold">${sess.title || 'Unknown'}</span>
                    <span class="text-xs text-black-100 block truncate group-hover:text-accent/80 transition-colors">${dateStr} via ${sess.deviceInfo || 'Web'}</span>
                  </div>
                  <span class="text-xs font-mono text-accent font-medium">${formatDuration(sess.timeListened)}</span>
                </div>
              `;
            }).join('');
          })
          .catch(err => {
            listContainer.innerHTML = `<p class="text-error text-center py-10 text-sm font-semibold">Failed to load recent sessions.</p>`;
          });
      };

    } else if (activeTab === 'library') {
      const activeLib = getLibrariesList().find(lib => lib.id === libraryId) || getActiveLibrary() || getLibrariesList()[0];
      const isBook = activeLib ? activeLib.mediaType === 'book' : true;

      const totalItems = stats.totalItems || 0;
      const overallHours = formatDurationLong(stats.totalDuration);
      const numAudioTracks = stats.numAudioTracks || stats.numAudioFiles || 0;
      const totalSize = formatBytes(stats.totalSize);
      const totalAuthors = stats.totalAuthors || 0;

      viewContentHtml = `
        <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-4">
          <h2 class="text-xl font-bold flex items-center space-x-2">
            <span class="material-symbols text-accent">library_books</span>
            <span>Library Stats</span>
          </h2>
          <select id="library-stats-select" class="bg-black-500 border border-black-400 text-white rounded px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent">
            ${getLibrariesList().map(lib => `
              <option value="${lib.id}" ${lib.id === libraryId ? 'selected' : ''}>${lib.name}</option>
            `).join('')}
          </select>
        </div>

        <!-- KPI Cards -->
        <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-5 gap-6">
          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">book</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Total Items</p>
              <h3 class="text-2xl font-bold text-white mt-1">${totalItems}</h3>
            </div>
          </div>

          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">schedule</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Total Duration</p>
              <h3 class="text-md font-bold text-white mt-1.5">${overallHours}</h3>
            </div>
          </div>

          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">music_note</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Audio Tracks</p>
              <h3 class="text-2xl font-bold text-white mt-1">${numAudioTracks}</h3>
            </div>
          </div>

          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">database</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Size</p>
              <h3 class="text-2xl font-bold text-white mt-1">${totalSize}</h3>
            </div>
          </div>

          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">${isBook ? 'person' : 'podcasts'}</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">${isBook ? 'Total Authors' : 'Episodes'}</p>
              <h3 class="text-2xl font-bold text-white mt-1">${isBook ? totalAuthors : numAudioTracks}</h3>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-${isBook ? 4 : 3} gap-8">
          <!-- Top Genres -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
            <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
              <span class="material-symbols text-accent text-xl">sell</span>
              <span>Top Genres</span>
            </h3>
            <div class="space-y-4 max-h-[350px] overflow-y-auto pr-2 no-scroll">
              ${(!stats.genresWithCount || stats.genresWithCount.length === 0) ? `
                <div class="text-center text-black-100 py-10 font-semibold text-sm">No genres found.</div>
              ` : stats.genresWithCount.slice(0, 10).map((item) => {
                const max = stats.genresWithCount[0].Count || 1;
                const pct = Math.round((item.Count / max) * 100);
                return `
                  <div class="space-y-1 text-sm">
                    <div class="flex justify-between items-center">
                      <span class="font-medium text-white truncate max-w-[80%]">${item.Genre}</span>
                      <span class="text-xs font-mono text-accent font-semibold">${item.Count}</span>
                    </div>
                    <div class="w-full bg-black-500 h-1.5 rounded-full overflow-hidden">
                      <div class="bg-accent h-full rounded-full" style="width: ${pct}%"></div>
                    </div>
                  </div>
                `;
              }).join('')}
            </div>
          </div>

          ${isBook ? `
            <!-- Top Authors -->
            <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
              <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
                <span class="material-symbols text-accent text-xl">person</span>
                <span>Top Authors</span>
              </h3>
              <div class="space-y-4 max-h-[350px] overflow-y-auto pr-2 no-scroll">
                ${(!stats.authorsWithCount || stats.authorsWithCount.length === 0) ? `
                  <div class="text-center text-black-100 py-10 font-semibold text-sm">No authors found.</div>
                ` : stats.authorsWithCount.slice(0, 10).map((item) => {
                  const max = stats.authorsWithCount[0].Count || 1;
                  const pct = Math.round((item.Count / max) * 100);
                  return `
                    <div class="space-y-1 text-sm">
                      <div class="flex justify-between items-center">
                        <span class="font-medium text-white truncate max-w-[80%] cursor-pointer hover:text-accent transition-colors block" onclick="window.navigateTo('/author/${item.ID}')">${item.Name}</span>
                        <span class="text-xs font-mono text-accent font-semibold">${item.Count}</span>
                      </div>
                      <div class="w-full bg-black-500 h-1.5 rounded-full overflow-hidden">
                        <div class="bg-accent h-full rounded-full" style="width: ${pct}%"></div>
                      </div>
                    </div>
                  `;
                }).join('')}
              </div>
            </div>
          ` : ''}

          <!-- Longest Items -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
            <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
              <span class="material-symbols text-accent text-xl">schedule</span>
              <span>Longest Items</span>
            </h3>
            <div class="space-y-4 max-h-[350px] overflow-y-auto pr-2 no-scroll">
              ${(!stats.longestItems || stats.longestItems.length === 0) ? `
                <div class="text-center text-black-100 py-10 font-semibold text-sm">No items found.</div>
              ` : stats.longestItems.map((item) => {
                const max = stats.longestItems[0].duration || 1;
                const pct = Math.round((item.duration / max) * 100);
                return `
                  <div class="space-y-1 text-sm">
                    <div class="flex justify-between items-center">
                      <span class="font-medium text-white truncate max-w-[70%] cursor-pointer hover:text-accent transition-colors block" onclick="window.navigateTo('/item/${item.id}')">${item.title}</span>
                      <span class="text-xs font-mono text-accent font-semibold">${formatDuration(item.duration)}</span>
                    </div>
                    <div class="w-full bg-black-500 h-1.5 rounded-full overflow-hidden">
                      <div class="bg-accent h-full rounded-full" style="width: ${pct}%"></div>
                    </div>
                  </div>
                `;
              }).join('')}
            </div>
          </div>

          <!-- Largest Items -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
            <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
              <span class="material-symbols text-accent text-xl">database</span>
              <span>Largest Items</span>
            </h3>
            <div class="space-y-4 max-h-[350px] overflow-y-auto pr-2 no-scroll">
              ${(!stats.largestItems || stats.largestItems.length === 0) ? `
                <div class="text-center text-black-100 py-10 font-semibold text-sm">No items found.</div>
              ` : stats.largestItems.map((item) => {
                const max = stats.largestItems[0].size || 1;
                const pct = Math.round((item.size / max) * 100);
                return `
                  <div class="space-y-1 text-sm">
                    <div class="flex justify-between items-center">
                      <span class="font-medium text-white truncate max-w-[70%] cursor-pointer hover:text-accent transition-colors block" onclick="window.navigateTo('/item/${item.id}')">${item.title}</span>
                      <span class="text-xs font-mono text-accent font-semibold">${formatBytes(item.size)}</span>
                    </div>
                    <div class="w-full bg-black-500 h-1.5 rounded-full overflow-hidden">
                      <div class="bg-accent h-full rounded-full" style="width: ${pct}%"></div>
                    </div>
                  </div>
                `;
              }).join('')}
            </div>
          </div>
        </div>
      `;

      // Hook library selector change event after render
      postRenderHook = () => {
        const selectEl = document.getElementById('library-stats-select');
        if (selectEl) {
          selectEl.onchange = (e) => {
            localStorage.setItem('activeLibraryId', e.target.value);
            fetchAndRender();
          };
        }
      };

    } else if (activeTab === 'server') {
      const totalTimeStr = formatDuration(stats.totalTime);
      const todayTimeStr = formatDuration(stats.today);
      const itemsList = Object.entries(stats.items || {}).map(([id, item]) => ({ id, ...item }));
      const uniqueItemsCount = itemsList.length;

      const maxDayOfWeek = Math.max(...Object.values(stats.dayOfWeek || {}), 1);
      const dayNames = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

      const monthsMap = {};
      Object.entries(stats.days || {}).forEach(([dateStr, seconds]) => {
        const monthStr = dateStr.substring(0, 7);
        monthsMap[monthStr] = (monthsMap[monthStr] || 0) + seconds;
      });
      const monthsList = Object.entries(monthsMap).sort((a, b) => a[0].localeCompare(b[0])).slice(-6);
      const maxMonthVal = Math.max(...monthsList.map(m => m[1]), 1);

      const dailyList = [];
      for (let i = 13; i >= 0; i--) {
        const d = new Date();
        d.setDate(d.getDate() - i);
        const dateStr = d.toISOString().split('T')[0];
        const val = stats.days ? (stats.days[dateStr] || 0) : 0;
        dailyList.push({ dateStr, val });
      }
      const maxDailyVal = Math.max(...dailyList.map(d => d.val), 1);

      const topAuthorsList = Object.entries(stats.topAuthors || {})
        .sort((a, b) => b[1] - a[1])
        .slice(0, 5);

      const topGenresList = Object.entries(stats.topGenres || {})
        .sort((a, b) => b[1] - a[1])
        .slice(0, 5);

      const topUsersList = Object.entries(stats.topUsers || {})
        .sort((a, b) => b[1] - a[1])
        .slice(0, 5);

      viewContentHtml = `
        <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-5 gap-6">
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
              <span class="material-symbols text-3xl">calendar_today</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Days Listened</p>
              <h3 class="text-2xl font-bold text-white mt-1">${stats.daysListened || 0}</h3>
            </div>
          </div>

          <div class="bg-primary border border-black-400 p-5 rounded-lg flex items-center space-x-4 shadow-md hover:shadow-lg transition-shadow">
            <div class="p-3 bg-accent/10 rounded-full text-accent flex items-center justify-center">
              <span class="material-symbols text-3xl">check_circle</span>
            </div>
            <div>
              <p class="text-xs text-black-100 uppercase tracking-wider font-semibold">Items Finished</p>
              <h3 class="text-2xl font-bold text-white mt-1">${stats.itemsFinished || 0}</h3>
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
        </div>

        <div class="space-y-8">
          <!-- Daily Listening Chart (14 days) -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md flex flex-col justify-between">
            <div>
              <h3 class="text-lg font-semibold mb-6 flex items-center space-x-2">
                <span class="material-symbols text-accent text-xl">calendar_view_month</span>
                <span>Daily Listening Trend (Last 14 Days)</span>
              </h3>
              <div class="flex items-end justify-between h-48 pt-4 px-2">
                ${dailyList.map(({ dateStr, val }) => {
                  const pct = Math.max((val / maxDailyVal) * 100, 3);
                  const formatted = formatDuration(val);
                  const dObj = new Date(dateStr + 'T00:00:00');
                  const label = dObj.toLocaleDateString(undefined, { month: 'numeric', day: 'numeric' });
                  return `
                    <div class="flex flex-col items-center flex-1 group">
                      <div class="relative w-full flex justify-center h-36 items-end">
                        <!-- Tooltip -->
                        <span class="absolute bottom-full mb-2 bg-black-500 text-xs px-2 py-1 rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap pointer-events-none z-10 text-center leading-relaxed">
                          ${formatted}<br/>${dateStr}
                        </span>
                        <!-- Bar -->
                        <div class="w-3 sm:w-6 bg-accent rounded-t transition-all duration-500 hover:bg-opacity-80" style="height: ${pct}%"></div>
                      </div>
                      <span class="text-[0.65rem] sm:text-xs text-black-100 mt-2 font-mono">${label}</span>
                    </div>
                  `;
                }).join('')}
              </div>
            </div>
          </div>

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
                    const pct = Math.max((val / maxDayOfWeek) * 100, 3);
                    const formatted = formatDuration(val);
                    return `
                      <div class="flex flex-col items-center flex-1 group">
                        <div class="relative w-full flex justify-center h-36 items-end">
                          <!-- Tooltip -->
                          <span class="absolute bottom-full mb-2 bg-black-500 text-xs px-2 py-1 rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap pointer-events-none z-10">
                            ${formatted}
                          </span>
                          <!-- Bar -->
                          <div class="w-6 sm:w-8 bg-accent rounded-t transition-all duration-500 hover:bg-opacity-80" style="height: ${pct}%"></div>
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
                    <div class="w-full text-center text-black-100 py-10 font-semibold">No monthly trends.</div>
                  ` : monthsList.map(([monthStr, val]) => {
                    const pct = Math.max((val / maxMonthVal) * 100, 3);
                    const formatted = formatDuration(val);
                    const date = new Date(monthStr + '-02');
                    const formattedMonth = isNaN(date.getTime()) ? monthStr : date.toLocaleDateString(undefined, { month: 'short', year: '2-digit' });
                    return `
                      <div class="flex flex-col items-center flex-1 group">
                        <div class="relative w-full flex justify-center h-36 items-end">
                          <!-- Tooltip -->
                          <span class="absolute bottom-full mb-2 bg-black-500 text-xs px-2 py-1 rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap pointer-events-none z-10">
                            ${formatted}
                          </span>
                          <!-- Bar -->
                          <div class="w-6 sm:w-8 bg-accent rounded-t transition-all duration-500 hover:bg-opacity-80" style="height: ${pct}%"></div>
                        </div>
                        <span class="text-xs text-black-100 mt-2 font-mono">${formattedMonth}</span>
                      </div>
                    `;
                  }).join('')}
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-8">
          <!-- Most Listened Items -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
            <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
              <span class="material-symbols text-accent text-xl">grade</span>
              <span>Most Listened Items</span>
            </h3>
            <div class="space-y-4 max-h-[250px] overflow-y-auto pr-2 no-scroll">
              ${itemsList.length === 0 ? `
                <div class="text-center text-black-100 py-10 font-semibold text-sm">No items in history.</div>
              ` : itemsList.sort((a, b) => b.timeListened - a.timeListened).slice(0, 5).map((item) => {
                const totalSec = stats.totalTime || 1;
                const progressPct = Math.round((item.timeListened / totalSec) * 100);
                return `
                  <div class="space-y-1">
                    <div class="flex justify-between items-center text-sm">
                      <div class="truncate max-w-[70%] cursor-pointer group" onclick="window.navigateTo('/item/${item.id}')">
                        <span class="font-medium text-white group-hover:text-accent transition-colors block truncate font-semibold">${item.title}</span>
                        ${item.author ? `<span class="text-xs text-black-100 block truncate group-hover:text-accent/80 transition-colors">by ${item.author}</span>` : ''}
                      </div>
                      <span class="text-xs font-mono text-accent font-semibold">${formatDuration(item.timeListened)}</span>
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
                <div class="text-center text-black-100 py-10 font-semibold text-sm">No author stats.</div>
              ` : topAuthorsList.map(([author, seconds]) => {
                const totalSec = stats.totalTime || 1;
                const progressPct = Math.round((seconds / totalSec) * 100);
                return `
                  <div class="space-y-1">
                    <div class="flex justify-between items-center text-sm">
                      <span class="font-medium text-white truncate max-w-[70%]">${author}</span>
                      <span class="text-xs font-mono text-accent font-semibold">${formatDuration(seconds)}</span>
                    </div>
                    <div class="w-full bg-black-500 h-1.5 rounded-full overflow-hidden">
                      <div class="bg-accent h-full rounded-full" style="width: ${progressPct}%"></div>
                    </div>
                  </div>
                `;
              }).join('')}
            </div>
          </div>

          <!-- Top Users -->
          <div class="bg-primary border border-black-400 p-6 rounded-lg shadow-md">
            <h3 class="text-lg font-semibold mb-4 flex items-center space-x-2">
              <span class="material-symbols text-accent text-xl">group</span>
              <span>Top Users</span>
            </h3>
            <div class="space-y-4 max-h-[250px] overflow-y-auto pr-2 no-scroll">
              ${topUsersList.length === 0 ? `
                <div class="text-center text-black-100 py-10 font-semibold text-sm">No user stats.</div>
              ` : topUsersList.map(([username, seconds]) => {
                const totalSec = stats.totalTime || 1;
                const progressPct = Math.round((seconds / totalSec) * 100);
                return `
                  <div class="space-y-1">
                    <div class="flex justify-between items-center text-sm">
                      <span class="font-medium text-white truncate max-w-[70%]">${username}</span>
                      <span class="text-xs font-mono text-accent font-semibold">${formatDuration(seconds)}</span>
                    </div>
                    <div class="w-full bg-black-500 h-1.5 rounded-full overflow-hidden">
                      <div class="bg-accent h-full rounded-full" style="width: ${progressPct}%"></div>
                    </div>
                  </div>
                `;
              }).join('')}
            </div>
          </div>
        </div>

        <!-- Playback Sessions Table -->
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
                  <th class="pb-3 pr-4">User</th>
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
          <div class="flex justify-between items-center mt-6 pt-4 border-t border-black-400 text-xs text-black-50 font-semibold">
            <span id="sessions-page-info">Showing page 1 of 1</span>
            <div class="flex space-x-2">
              <button id="sessions-prev-btn" class="bg-black-500 hover:bg-black-400 text-white px-3 py-1.5 rounded disabled:opacity-50 font-semibold">Previous</button>
              <button id="sessions-next-btn" class="bg-black-500 hover:bg-black-400 text-white px-3 py-1.5 rounded disabled:opacity-50 font-semibold">Next</button>
            </div>
          </div>
        </div>
      `;

      // Set up sessions paginator
      let currentPage = 0;
      const itemsPerPage = 10;

      const renderSessionsTable = async (page) => {
        const tableBody = document.getElementById('sessions-table-body');
        if (!tableBody) return;

        tableBody.innerHTML = `<tr><td colspan="6" class="py-10 text-center"><div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div></td></tr>`;

        try {
          const sessionsUrl = `/api/server-listening-sessions?page=${page}&itemsPerPage=${itemsPerPage}`;
          const paginated = await request('GET', sessionsUrl);
          const sessions = paginated.sessions || [];
          const total = paginated.total || 0;
          const totalPages = Math.max(Math.ceil(total / itemsPerPage), 1);

          document.getElementById('sessions-page-info').textContent = `Showing page ${page + 1} of ${totalPages} (${total} total sessions)`;
          document.getElementById('sessions-prev-btn').disabled = (page === 0);
          document.getElementById('sessions-next-btn').disabled = (page >= totalPages - 1);

          if (sessions.length === 0) {
            tableBody.innerHTML = `<tr><td colspan="6" class="py-10 text-center text-black-100 font-semibold">No sessions recorded yet.</td></tr>`;
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
                  <span class="group-hover:text-accent transition-colors block truncate font-semibold">${sess.title || 'Unknown Item'}</span>
                  ${sess.author ? `<span class="text-xs text-black-100 block truncate group-hover:text-accent/80 transition-colors">by ${sess.author}</span>` : ''}
                </td>
                <td class="py-3 pr-4 text-black-50 font-semibold">${sess.username || 'Unknown User'}</td>
                <td class="py-3 pr-4 text-black-50 font-semibold">${dateStr}</td>
                <td class="py-3 pr-4 text-black-50 font-semibold">${sess.deviceInfo || 'Web Client'}</td>
                <td class="py-3 pr-4"><span class="bg-black-500 px-2 py-0.5 rounded text-xs border border-black-400 font-mono font-semibold">${sess.playMethod || 'HLS'}</span></td>
                <td class="py-3 text-right font-mono font-medium text-accent">${formatDuration(sess.timeListened)}</td>
              </tr>
            `;
          }).join('');
        } catch (err) {
          tableBody.innerHTML = `<tr><td colspan="6" class="py-10 text-center text-error font-semibold">Failed to load playback sessions.</td></tr>`;
        }
      };

      postRenderHook = () => {
        const prevBtn = document.getElementById('sessions-prev-btn');
        const nextBtn = document.getElementById('sessions-next-btn');
        if (prevBtn) {
          prevBtn.onclick = () => {
            if (currentPage > 0) {
              currentPage--;
              renderSessionsTable(currentPage);
            }
          };
        }
        if (nextBtn) {
          nextBtn.onclick = () => {
            currentPage++;
            renderSessionsTable(currentPage);
          };
        }
        renderSessionsTable(0);
      };
    }

    container.innerHTML = `
      <div class="p-6 max-w-6xl mx-auto space-y-8 text-white">
        ${tabsHtml}
        ${viewContentHtml}
      </div>
    `;

    if (postRenderHook) postRenderHook();

    // Hook tab switches
    const tabMy = document.getElementById('tab-my-stats');
    if (tabMy) {
      tabMy.onclick = () => {
        activeTab = 'my';
        fetchAndRender();
      };
    }

    const tabLib = document.getElementById('tab-library-stats');
    if (tabLib) {
      tabLib.onclick = () => {
        activeTab = 'library';
        fetchAndRender();
      };
    }

    const tabServer = document.getElementById('tab-server-stats');
    if (tabServer) {
      tabServer.onclick = () => {
        activeTab = 'server';
        fetchAndRender();
      };
    }
  };

  await fetchAndRender();
}
