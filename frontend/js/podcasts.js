import { request } from './api.js';
import { playItem } from './player.js';

export async function loadPodcastLatestView(libraryId) {
  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Latest Episodes';
  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = 'Loading...';

  container.innerHTML = `<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent mx-auto mt-20"></div>`;

  try {
    const res = await request('GET', `/api/libraries/${libraryId}/items?limit=100&minified=1`);
    const podcasts = res.results || [];
    
    if (podcasts.length === 0) {
      if (bookCount) bookCount.textContent = '0 Episodes';
      container.innerHTML = `
        <div class="text-center py-20 bg-primary border border-black-400 rounded-md">
          <span class="material-symbols text-5xl text-black-100 mb-2">podcasts</span>
          <p class="text-black-50 mb-4">No podcasts found in this library. Click "Add" in the sidebar to subscribe to one!</p>
        </div>
      `;
      return;
    }

    // Fetch full podcast details concurrently to get episodes
    const episodes = [];
    await Promise.all(podcasts.map(async (pod) => {
      try {
        const fullPod = await request('GET', `/api/items/${pod.id}`);
        if (fullPod && fullPod.media && fullPod.media.episodes) {
          fullPod.media.episodes.forEach(ep => {
            episodes.push({
              ...ep,
              podcastId: pod.id,
              podcastItem: fullPod,
              podcastTitle: fullPod.media?.metadata?.title || 'Unknown Podcast',
              podcastCover: fullPod.media?.coverPath || null
            });
          });
        }
      } catch (err) {
        console.warn(`Failed to fetch podcast detail for ${pod.id}:`, err);
      }
    }));

    // Sort episodes by pubDate descending
    episodes.sort((a, b) => {
      const dateA = new Date(a.pubDate || a.publishedAt || 0);
      const dateB = new Date(b.pubDate || b.publishedAt || 0);
      return dateB - dateA;
    });

    if (bookCount) bookCount.textContent = `${episodes.length} Episodes`;

    if (episodes.length === 0) {
      container.innerHTML = `
        <div class="text-center py-20 bg-primary border border-black-400 rounded-md">
          <span class="material-symbols text-5xl text-black-100 mb-2">podcasts</span>
          <p class="text-black-50 mb-4">No episodes found. Trigger a scan or sync on your podcasts to load their feeds.</p>
        </div>
      `;
      return;
    }

    let html = `
      <div class="p-6 space-y-4">
        <h2 class="text-xl font-bold text-white mb-6">Recent Podcast Episodes</h2>
        <div class="space-y-3 max-w-4xl">
    `;

    episodes.forEach((ep, idx) => {
      const isDownloaded = ep.audioFile && ep.audioFile.metadata && ep.audioFile.metadata.path;
      const token = localStorage.getItem('token');
      const ts = ep.podcastItem.updatedAt || ep.podcastItem.addedAt || Date.now();
      const coverUrl = ep.podcastCover ? resolvePath(`/api/items/${ep.podcastId}/cover?token=${token}&ts=${ts}`) : null;
      
      const coverHtml = coverUrl 
        ? `<img src="${coverUrl}" class="w-12 h-12 bg-black-500 rounded border border-black-400 object-cover flex-shrink-0" onerror="this.onerror=null; this.src='assets/images/logo.png'">`
        : `<div class="w-12 h-12 bg-black-500 rounded border border-black-400 flex items-center justify-center flex-shrink-0"><span class="material-symbols text-xl text-black-200">podcasts</span></div>`;

      html += `
        <div class="flex items-center justify-between p-4 hover:bg-black-500/40 rounded-md border border-black-400/50 bg-primary/20 transition-colors text-sm">
          <div class="flex items-center space-x-3 truncate flex-grow mr-4">
            ${coverHtml}
            <div class="truncate">
              <p class="font-semibold text-white truncate text-base">${escapeHtml(ep.title)}</p>
              <p class="text-xs text-black-100 mt-0.5">${escapeHtml(ep.podcastTitle)} ${ep.pubDate ? `• ${escapeHtml(ep.pubDate)}` : ''}</p>
            </div>
          </div>
          <div class="flex items-center space-x-2 flex-shrink-0">
            ${isDownloaded ? `
              <button class="play-episode-btn flex items-center space-x-1 bg-accent text-primary px-3 py-1.5 rounded font-bold hover:opacity-90 transition-opacity" data-idx="${idx}">
                <span class="material-symbols text-sm font-bold">play_arrow</span>
                <span>Play</span>
              </button>
            ` : `
              <button class="download-episode-btn flex items-center space-x-1 bg-black-400 hover:bg-black-300 border border-black-300 text-white px-3 py-1.5 rounded font-bold transition-colors" data-pod-id="${ep.podcastId}" data-ep-id="${ep.id}">
                <span class="material-symbols text-sm">download</span>
                <span>Download</span>
              </button>
            `}
          </div>
        </div>
      `;
    });

    html += `
        </div>
      </div>
    `;

    container.innerHTML = html;

    // Attach listeners
    container.querySelectorAll('.play-episode-btn').forEach(btn => {
      btn.onclick = () => {
        const idx = parseInt(btn.getAttribute('data-idx'), 10);
        const ep = episodes[idx];
        const mockItem = {
          ...ep.podcastItem,
          media: {
            ...ep.podcastItem.media,
            audioFiles: [ep.audioFile],
            duration: ep.duration || 0,
            metadata: {
              ...ep.podcastItem.media.metadata,
              title: ep.title
            }
          }
        };
        playItem(mockItem, 0);
      };
    });

    container.querySelectorAll('.download-episode-btn').forEach(btn => {
      btn.onclick = async () => {
        const podId = btn.getAttribute('data-pod-id');
        const epId = btn.getAttribute('data-ep-id');
        btn.disabled = true;
        btn.innerHTML = `<span class="animate-spin text-white material-symbols text-xs mr-1">sync</span><span>Queueing...</span>`;
        try {
          await request('POST', `/api/podcasts/${podId}/download-episodes`, [epId]);
          if (window.showToast) window.showToast('Download started in background', 'success');
          btn.className = "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 text-xs font-bold px-3 py-1.5 rounded cursor-default focus:outline-none";
          btn.innerHTML = `<span class="material-symbols text-xs mr-1">sync_saved_locally</span><span>Queued</span>`;
          btn.onclick = null;
        } catch (err) {
          console.error(err);
          if (window.showToast) window.showToast('Failed to queue download: ' + err.message, 'error');
          btn.disabled = false;
          btn.innerHTML = `<span class="material-symbols text-sm">download</span><span>Download</span>`;
        }
      };
    });

  } catch (err) {
    console.error('Failed to load latest episodes:', err);
    if (bookCount) bookCount.textContent = 'Error';
    container.innerHTML = `<p class="text-red-400 text-center py-20">Error loading episodes: ${escapeHtml(err.message)}</p>`;
  }
}

export async function loadPodcastAddView(libraryId) {
  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Add Podcast';
  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = '';

  container.innerHTML = `
    <div class="p-6 max-w-4xl space-y-8">
      <div>
        <h2 class="text-xl font-bold text-white mb-1">Add Podcast</h2>
        <p class="text-xs text-black-100">Subscribe to a new podcast via iTunes search or feed RSS URL.</p>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <!-- Search Card -->
        <div class="bg-primary border border-black-400 p-5 rounded flex flex-col space-y-4 shadow-md">
          <h3 class="font-bold text-sm text-white flex items-center space-x-1.5">
            <span class="material-symbols text-sm text-accent">search</span>
            <span>Search iTunes</span>
          </h3>
          <div class="flex space-x-2">
            <input id="podcast-search-input" type="text" placeholder="Podcast Title or Author..." class="flex-1 bg-black-500 border border-black-400 text-white rounded px-3 py-2 text-xs focus:outline-none focus:border-accent">
            <button id="podcast-search-btn" class="bg-accent hover:bg-accent/80 text-primary font-bold px-4 py-2 rounded text-xs transition-colors flex items-center justify-center space-x-1.5 focus:outline-none focus:ring-2 focus:ring-accent/50">
              <span class="material-symbols text-sm">search</span>
              <span>Search</span>
            </button>
          </div>
          <div id="podcast-search-results" class="space-y-2 max-h-96 overflow-y-auto no-scroll pt-2">
            <p class="text-xs text-black-200 text-center py-10">Search results will appear here.</p>
          </div>
        </div>

        <!-- Direct Subscribe Card -->
        <div class="bg-primary border border-black-400 p-5 rounded flex flex-col space-y-4 h-fit shadow-md">
          <h3 class="font-bold text-sm text-white flex items-center space-x-1.5">
            <span class="material-symbols text-sm text-accent">rss_feed</span>
            <span>Subscribe via RSS URL</span>
          </h3>
          <div class="space-y-3">
            <input id="podcast-rss-input" type="text" placeholder="https://example.com/feed.xml" class="w-full bg-black-500 border border-black-400 text-white rounded px-3 py-2 text-xs focus:outline-none focus:border-accent">
            <button id="podcast-rss-subscribe-btn" class="w-full bg-accent hover:bg-accent/80 text-primary font-bold py-2 rounded text-xs transition-colors flex items-center justify-center space-x-1.5 focus:outline-none focus:ring-2 focus:ring-accent/50">
              <span class="material-symbols text-sm">rss_feed</span>
              <span>Subscribe</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  `;

  const searchInput = document.getElementById('podcast-search-input');
  const searchBtn = document.getElementById('podcast-search-btn');
  const resultsContainer = document.getElementById('podcast-search-results');
  const rssInput = document.getElementById('podcast-rss-input');
  const rssSubscribeBtn = document.getElementById('podcast-rss-subscribe-btn');

  // iTunes Search Handler
  const performSearch = async () => {
    const query = searchInput.value.trim();
    if (!query) return;

    searchBtn.disabled = true;
    searchBtn.innerHTML = `
      <div class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary mr-1.5 inline-block align-middle"></div>
      <span class="align-middle">Searching...</span>
    `;
    resultsContainer.innerHTML = `
      <div class="flex flex-col items-center justify-center py-16 space-y-2">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
        <p class="text-xs text-black-200">Querying iTunes Directory...</p>
      </div>
    `;

    try {
      const results = await request('GET', `/api/search/podcast?term=${encodeURIComponent(query)}`);
      
      if (!results || results.length === 0) {
        resultsContainer.innerHTML = `<p class="text-xs text-black-150 text-center py-10">No podcasts found on iTunes.</p>`;
        return;
      }

      resultsContainer.innerHTML = results.map((pod, idx) => {
        const coverHtml = pod.coverUrl 
          ? `<img src="${escapeHtml(pod.coverUrl)}" class="w-12 h-12 bg-black-500 rounded border border-black-400 object-cover flex-shrink-0" alt="">`
          : `<div class="w-12 h-12 bg-black-500 rounded border border-black-400 flex items-center justify-center flex-shrink-0"><span class="material-symbols text-xl text-black-200">podcasts</span></div>`;

        const authorText = pod.authors && pod.authors.length > 0 ? pod.authors.join(', ') : 'Unknown Author';
        
        return `
          <div class="flex items-center justify-between p-2.5 hover:bg-black-500/50 rounded border border-black-400/40 transition-colors text-xs bg-primary/10 hover:border-black-400">
            <div class="flex items-center space-x-3 min-w-0 mr-2">
              ${coverHtml}
              <div class="min-w-0">
                <p class="font-bold text-white truncate">${escapeHtml(pod.title)}</p>
                <p class="text-[10px] text-black-100 truncate mt-0.5">${escapeHtml(authorText)}</p>
              </div>
            </div>
            <button class="subscribe-search-btn bg-accent hover:bg-accent/80 text-primary font-bold px-3 py-1.5 rounded text-[11px] transition-colors flex-shrink-0 focus:outline-none focus:ring-2 focus:ring-accent/50" data-idx="${idx}">
              Subscribe
            </button>
          </div>
        `;
      }).join('');

      // Hook Subscribe buttons
      resultsContainer.querySelectorAll('.subscribe-search-btn').forEach(btn => {
        btn.onclick = async () => {
          const idx = parseInt(btn.getAttribute('data-idx'), 10);
          const pod = results[idx];
          btn.disabled = true;
          btn.innerHTML = `
            <div class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary mr-1 inline-block align-middle"></div>
            <span class="align-middle">Subscribing...</span>
          `;
          try {
            await request('POST', `/api/podcasts`, {
              libraryId: libraryId,
              feedUrl: pod.feedUrl
            });
            if (window.showToast) window.showToast('Subscribed to podcast successfully!', 'success');
            btn.className = "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 text-[11px] font-bold px-3 py-1.5 rounded cursor-default flex items-center space-x-1";
            btn.innerHTML = `
              <span class="material-symbols text-[13px]">check</span>
              <span>Subscribed</span>
            `;
          } catch (err) {
            console.error(err);
            if (window.showToast) window.showToast('Subscription failed: ' + err.message, 'error');
            btn.disabled = false;
            btn.textContent = 'Subscribe';
          }
        };
      });

    } catch (err) {
      console.error(err);
      resultsContainer.innerHTML = `<p class="text-xs text-red-400 text-center py-10">Search failed: ${escapeHtml(err.message)}</p>`;
    } finally {
      searchBtn.disabled = false;
      searchBtn.innerHTML = `
        <span class="material-symbols text-sm">search</span>
        <span>Search</span>
      `;
    }
  };

  searchBtn.onclick = performSearch;
  searchInput.onkeydown = (e) => {
    if (e.key === 'Enter') performSearch();
  };

  // Direct RSS Subscribe handler
  rssSubscribeBtn.onclick = async () => {
    const feedUrl = rssInput.value.trim();
    if (!feedUrl) return;

    rssSubscribeBtn.disabled = true;
    rssSubscribeBtn.innerHTML = `
      <div class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary mr-1.5 inline-block align-middle"></div>
      <span class="align-middle">Subscribing...</span>
    `;

    try {
      await request('POST', `/api/podcasts`, {
        libraryId: libraryId,
        feedUrl: feedUrl
      });
      if (window.showToast) window.showToast('Subscribed to podcast feed successfully!', 'success');
      rssInput.value = '';
    } catch (err) {
      console.error(err);
      if (window.showToast) window.showToast('Subscription failed: ' + err.message, 'error');
    } finally {
      rssSubscribeBtn.disabled = false;
      rssSubscribeBtn.innerHTML = `
        <span class="material-symbols text-sm">rss_feed</span>
        <span>Subscribe</span>
      `;
    }
  };
}

export async function loadPodcastDownloadQueueView(libraryId) {
  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Download Queue';
  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = 'Loading...';

  container.innerHTML = `<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent mx-auto mt-20"></div>`;

  const renderQueue = async () => {
    try {
      const res = await request('GET', '/api/tasks?include=queue');
      const tasks = res.tasks || [];
      
      if (bookCount) {
        bookCount.textContent = tasks.length === 1 ? '1 Active Task' : `${tasks.length} Active Tasks`;
      }

      if (tasks.length === 0) {
        container.innerHTML = `
          <div class="p-6 space-y-4">
            <h2 class="text-xl font-bold text-white mb-6">Podcast Download Queue</h2>
            <div class="text-center py-20 bg-primary border border-black-400 rounded-md">
              <span class="material-symbols text-5xl text-black-100 mb-2">download</span>
              <p class="text-black-50">No downloads currently active or queued.</p>
            </div>
          </div>
        `;
        return;
      }

      let html = `
        <div class="p-6 space-y-6">
          <div class="flex justify-between items-center max-w-4xl">
            <h2 class="text-xl font-bold text-white">Podcast Download Queue</h2>
            <button id="cancel-all-tasks-btn" class="bg-red-600/20 hover:bg-red-600/30 border border-red-500/30 text-red-400 font-bold px-4 py-2 rounded text-xs transition-colors">
              Cancel All Tasks
            </button>
          </div>
          <div class="space-y-4 max-w-4xl">
      `;

      tasks.forEach(task => {
        const progress = task.progress || 0;
        const speed = task.speed || '';
        const sizeStr = task.size || '';
        const title = task.title || task.name || 'Podcast Episode Download';
        const isPaused = task.status === 'paused';

        html += `
          <div class="bg-primary/20 border border-black-400/50 rounded-md p-4 space-y-3">
            <div class="flex justify-between items-start">
              <div class="min-w-0">
                <p class="font-bold text-white text-sm truncate">${escapeHtml(title)}</p>
                <p class="text-xs text-black-100 mt-0.5">Status: <span class="capitalize text-accent">${escapeHtml(task.status)}</span> ${speed ? `| Speed: ${escapeHtml(speed)}` : ''} ${sizeStr ? `| Size: ${escapeHtml(sizeStr)}` : ''}</p>
              </div>
              <div class="flex space-x-2 flex-shrink-0">
                <button class="pause-resume-task-btn bg-black-400 hover:bg-black-300 text-white font-bold px-3 py-1.5 rounded text-xs transition-colors" data-id="${task.id}" data-action="${isPaused ? 'resume' : 'pause'}">
                  ${isPaused ? 'Resume' : 'Pause'}
                </button>
                <button class="cancel-task-btn bg-red-600/20 hover:bg-red-600/30 border border-red-500/30 text-red-400 font-bold px-3 py-1.5 rounded text-xs transition-colors" data-id="${task.id}">
                  Cancel
                </button>
              </div>
            </div>

            <!-- Progress Bar -->
            <div class="w-full bg-black-600 rounded-full h-2 relative overflow-hidden">
              <div class="bg-accent h-full rounded-full transition-all duration-300" style="width: ${progress}%"></div>
            </div>
            <div class="flex justify-end text-[10px] text-black-100">
              <span>${progress}% Complete</span>
            </div>
          </div>
        `;
      });

      html += `
          </div>
        </div>
      `;

      container.innerHTML = html;

      // Bind Cancel All
      const cancelAllBtn = document.getElementById('cancel-all-tasks-btn');
      if (cancelAllBtn) {
        cancelAllBtn.onclick = async () => {
          const confirmed = await window.showConfirm(
            'Cancel Downloads',
            'Are you sure you want to cancel all downloads?',
            'Cancel All',
            'Cancel'
          );
          if (!confirmed) return;
          try {
            await request('POST', '/api/tasks/cancel-all');
            if (window.showToast) window.showToast('Cancelled all download tasks', 'success');
            renderQueue();
          } catch (err) {
            console.error(err);
            if (window.showToast) window.showToast('Failed to cancel all tasks: ' + err.message, 'error');
          }
        };
      }

      // Bind Pause/Resume
      container.querySelectorAll('.pause-resume-task-btn').forEach(btn => {
        btn.onclick = async () => {
          const taskId = btn.getAttribute('data-id');
          const action = btn.getAttribute('data-action');
          try {
            await request('POST', `/api/tasks/${taskId}/${action}`);
            if (window.showToast) window.showToast(`Task ${action}d successfully`, 'success');
            renderQueue();
          } catch (err) {
            console.error(err);
            if (window.showToast) window.showToast(`Failed to ${action} task: ` + err.message, 'error');
          }
        };
      });

      // Bind Cancel Task
      container.querySelectorAll('.cancel-task-btn').forEach(btn => {
        btn.onclick = async () => {
          const taskId = btn.getAttribute('data-id');
          try {
            await request('POST', `/api/tasks/${taskId}/cancel`);
            if (window.showToast) window.showToast('Task cancelled successfully', 'success');
            renderQueue();
          } catch (err) {
            console.error(err);
            if (window.showToast) window.showToast('Failed to cancel task: ' + err.message, 'error');
          }
        };
      });

    } catch (err) {
      console.error(err);
      if (bookCount) bookCount.textContent = 'Error';
      container.innerHTML = `<p class="text-red-400 text-center py-20">Error loading download tasks: ${escapeHtml(err.message)}</p>`;
    }
  };

  renderQueue();
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

import { getActiveLibrary } from './library.js';

export async function downloadOPML(libraryId) {
  try {
    const opmlText = await request('GET', `/api/podcasts/opml/export?libraryId=${libraryId}`);
    const blob = new Blob([opmlText], { type: 'application/xml' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'podcasts.opml';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    if (window.showToast) window.showToast('OPML file downloaded successfully', 'success');
  } catch (err) {
    if (window.showToast) window.showToast('Failed to export OPML: ' + err.message, 'error');
  }
}

export function setupPodcastExportButton(libraryId) {
  const opmlBtn = document.getElementById('opml-btn');
  if (!opmlBtn) return;

  const lib = getActiveLibrary();
  const isPodcast = lib && (lib.mediaType === 'podcast' || lib.icon === 'podcasts');

  let exportBtn = document.getElementById('export-opml-btn');

  if (isPodcast) {
    if (!exportBtn) {
      exportBtn = document.createElement('button');
      exportBtn.id = 'export-opml-btn';
      exportBtn.className = 'flex items-center space-x-1 hover:bg-black-500 px-2.5 py-1.5 rounded text-black-50 hover:text-white border border-transparent hover:border-black-300';
      exportBtn.title = 'Export OPML';
      exportBtn.innerHTML = `
        <span class="material-symbols text-base">download</span>
        <span>Export OPML</span>
      `;
      opmlBtn.parentNode.insertBefore(exportBtn, opmlBtn.nextSibling);
    }
    if (opmlBtn.classList.contains('hidden')) {
      exportBtn.classList.add('hidden');
    } else {
      exportBtn.classList.remove('hidden');
    }
    exportBtn.onclick = () => {
      downloadOPML(libraryId);
    };
  } else {
    if (exportBtn) {
      exportBtn.classList.add('hidden');
    }
  }
}

// Global listener & observer to keep button state updated
window.addEventListener('library-changed', (e) => {
  const libraryId = e.detail.libraryId;
  setupPodcastExportButton(libraryId);
});

// Observe opml-btn class changes to sync visibility
const opmlBtnObserver = new MutationObserver((mutations) => {
  mutations.forEach((mutation) => {
    if (mutation.attributeName === 'class') {
      const opmlBtn = mutation.target;
      const exportBtn = document.getElementById('export-opml-btn');
      if (exportBtn) {
        if (opmlBtn.classList.contains('hidden')) {
          exportBtn.classList.add('hidden');
        } else {
          const lib = getActiveLibrary();
          if (lib && (lib.mediaType === 'podcast' || lib.icon === 'podcasts')) {
            exportBtn.classList.remove('hidden');
          }
        }
      }
    }
  });
});

document.addEventListener('DOMContentLoaded', () => {
  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) {
    opmlBtnObserver.observe(opmlBtn, { attributes: true });
  }
});

// Since DOMContentLoaded might have already fired, check and attach immediately as well
const opmlBtn = document.getElementById('opml-btn');
if (opmlBtn) {
  opmlBtnObserver.observe(opmlBtn, { attributes: true });
}
