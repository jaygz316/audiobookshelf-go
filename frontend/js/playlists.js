import { request, resolvePath } from './api.js';
import { playItem, playItems } from './player.js';
import { loadItemDetails } from './itemDetails.js';

let currentSearch = '';
let currentSort = 'name'; // 'name', 'numBooks'
let currentDesc = false;
let allPlaylists = [];

export async function loadPlaylists(libraryId) {
  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Playlists';
  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = 'Loading...';

  container.innerHTML = `<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent mx-auto mt-20"></div>`;

  try {
    const res = await request('GET', `/api/libraries/${libraryId}/playlists`);
    allPlaylists = res.results || [];

    container.innerHTML = `
      <div class="p-6 space-y-6 text-left">
        <div class="flex justify-between items-center">
          <h2 class="text-xl font-bold">Your Playlists</h2>
          <button id="create-playlist-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity flex items-center space-x-1.5 text-sm">
            <span class="material-symbols text-lg">add</span>
            <span>Create Playlist</span>
          </button>
        </div>

        <!-- Controls Toolbar -->
        <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-4 bg-black-600/40 p-4 rounded-lg border border-black-600/30 animate-fade-in">
          <!-- Search input -->
          <div class="relative flex-grow max-w-md">
            <span class="material-symbols absolute left-3 top-2.5 text-black-200 text-lg">search</span>
            <input type="text" id="playlists-search" placeholder="Search playlists..." value="${escapeHtml(currentSearch)}"
              class="w-full bg-black-500 text-white pl-10 pr-10 py-2 rounded-lg border border-black-300 focus:outline-none focus:border-accent text-sm transition-colors">
            ${currentSearch ? `
              <button id="playlists-search-clear-btn" class="absolute right-3 top-2.5 text-black-200 hover:text-white transition-colors focus:outline-none" title="Clear Search">
                <span class="material-symbols text-lg">close</span>
              </button>
            ` : ''}
          </div>

          <!-- Sort and Order controls -->
          <div class="flex items-center gap-3">
            <label class="text-xs font-semibold text-black-100 uppercase tracking-wider">Sort by:</label>
            <select id="playlists-sort-select" class="bg-black-500 border border-black-300 text-white text-xs rounded px-3 py-1.5 focus:outline-none cursor-pointer">
              <option value="name" ${currentSort === 'name' ? 'selected' : ''}>Name</option>
              <option value="numBooks" ${currentSort === 'numBooks' ? 'selected' : ''}>Book Count</option>
            </select>
            <button id="playlists-direction-btn" class="p-1.5 bg-black-500 hover:bg-black-400 border border-black-300 rounded text-white flex items-center justify-center transition-colors" title="Toggle Sort Order">
              <span class="material-symbols text-lg">${currentDesc ? 'arrow_downward' : 'arrow_upward'}</span>
            </button>
          </div>
        </div>

        <div id="playlists-grid" class="library-grid w-full">
          <!-- Playlist Cards -->
        </div>

        <div id="playlists-empty" class="text-center py-20 bg-primary border border-black-400 rounded-md hidden">
          <span class="material-symbols text-5xl text-black-100 mb-2">playlist_play</span>
          <p class="text-black-50 mb-4">No playlists found. Create your first playlist to organize your listening queue!</p>
          <button id="create-first-playlist-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity text-sm">Create Playlist</button>
        </div>
      </div>
    `;

    const updateGrid = () => {
      let filtered = [...allPlaylists];

      if (currentSearch) {
        const searchLower = currentSearch.toLowerCase();
        filtered = filtered.filter(p =>
          (p.name && p.name.toLowerCase().includes(searchLower)) ||
          (p.description && p.description.toLowerCase().includes(searchLower))
        );
      }

      filtered.sort((a, b) => {
        let comparison = 0;
        if (currentSort === 'numBooks') {
          const countA = (a.items || a.itemIds || []).length;
          const countB = (b.items || b.itemIds || []).length;
          comparison = countA - countB;
        }

        if (comparison === 0) {
          const nameA = (a.name || '').toLowerCase();
          const nameB = (b.name || '').toLowerCase();
          comparison = nameA.localeCompare(nameB);
        }

        return currentDesc ? -comparison : comparison;
      });

      if (bookCount) {
        bookCount.textContent = filtered.length === 1 ? '1 Playlist' : `${filtered.length} Playlists`;
      }

      renderPlaylistsGrid(filtered, libraryId);

      // Wire up empty state button if needed
      const emptyBtn = document.getElementById('create-first-playlist-btn');
      if (emptyBtn) emptyBtn.onclick = showCreateModal;
    };

    const showCreateModal = () => triggerCreatePlaylistModal(libraryId);
    document.getElementById('create-playlist-btn').onclick = showCreateModal;

    // Listeners
    const searchInput = document.getElementById('playlists-search');
    if (searchInput) {
      searchInput.addEventListener('input', (e) => {
        currentSearch = e.target.value;
        loadPlaylists(libraryId); // reload structure to reflect show/hide clear btn
      });
    }

    const clearSearchBtn = document.getElementById('playlists-search-clear-btn');
    if (clearSearchBtn) {
      clearSearchBtn.addEventListener('click', () => {
        currentSearch = '';
        loadPlaylists(libraryId);
      });
    }

    const sortSelect = document.getElementById('playlists-sort-select');
    if (sortSelect) {
      sortSelect.addEventListener('change', (e) => {
        currentSort = e.target.value;
        updateGrid();
      });
    }

    const directionBtn = document.getElementById('playlists-direction-btn');
    if (directionBtn) {
      directionBtn.addEventListener('click', () => {
        currentDesc = !currentDesc;
        const icon = directionBtn.querySelector('.material-symbols');
        if (icon) {
          icon.textContent = currentDesc ? 'arrow_downward' : 'arrow_upward';
        }
        updateGrid();
      });
    }

    updateGrid();

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm p-6">Failed to load playlists: ${err.message}</p>`;
  }
}


function renderCoversPreview(bookIds, token) {
  if (!bookIds || bookIds.length === 0) {
    return `<span class="material-symbols text-4xl text-accent/80">playlist_play</span>`;
  }
  
  if (bookIds.length === 1) {
    return `<img src="${resolvePath(`/api/items/${bookIds[0]}/cover?token=${token}&width=200`)}" class="w-full h-full object-cover" alt="Cover" onerror="this.onerror=null; this.src='assets/images/book_placeholder.jpg';">`;
  }
  
  if (bookIds.length === 2) {
    return `
      <div class="grid grid-cols-2 w-full h-full gap-0.5">
        <img src="${resolvePath(`/api/items/${bookIds[0]}/cover?token=${token}&width=120`)}" class="w-full h-full object-cover" alt="Cover 1" onerror="this.style.display='none'">
        <img src="${resolvePath(`/api/items/${bookIds[1]}/cover?token=${token}&width=120`)}" class="w-full h-full object-cover" alt="Cover 2" onerror="this.style.display='none'">
      </div>
    `;
  }
  
  if (bookIds.length === 3) {
    return `
      <div class="grid grid-cols-2 grid-rows-2 w-full h-full gap-0.5">
        <img src="${resolvePath(`/api/items/${bookIds[0]}/cover?token=${token}&width=100`)}" class="w-full h-full object-cover row-span-2" alt="Cover 1" onerror="this.style.display='none'">
        <img src="${resolvePath(`/api/items/${bookIds[1]}/cover?token=${token}&width=100`)}" class="w-full h-full object-cover" alt="Cover 2" onerror="this.style.display='none'">
        <img src="${resolvePath(`/api/items/${bookIds[2]}/cover?token=${token}&width=100`)}" class="w-full h-full object-cover" alt="Cover 3" onerror="this.style.display='none'">
      </div>
    `;
  }

  // 4 or more
  return `
    <div class="grid grid-cols-2 grid-rows-2 w-full h-full gap-0.5">
      <img src="${resolvePath(`/api/items/${bookIds[0]}/cover?token=${token}&width=100`)}" class="w-full h-full object-cover" alt="Cover 1" onerror="this.style.display='none'">
      <img src="${resolvePath(`/api/items/${bookIds[1]}/cover?token=${token}&width=100`)}" class="w-full h-full object-cover" alt="Cover 2" onerror="this.style.display='none'">
      <img src="${resolvePath(`/api/items/${bookIds[2]}/cover?token=${token}&width=100`)}" class="w-full h-full object-cover" alt="Cover 3" onerror="this.style.display='none'">
      <img src="${resolvePath(`/api/items/${bookIds[3]}/cover?token=${token}&width=100`)}" class="w-full h-full object-cover" alt="Cover 4" onerror="this.style.display='none'">
    </div>
  `;
}

function renderPlaylistsGrid(playlists, libraryId) {
  const grid = document.getElementById('playlists-grid');
  const emptyState = document.getElementById('playlists-empty');
  if (!grid) return;

  grid.innerHTML = '';
  if (playlists.length === 0) {
    emptyState.classList.remove('hidden');
    return;
  }
  emptyState.classList.add('hidden');

  const token = localStorage.getItem('token');

  playlists.forEach(p => {
    const card = document.createElement('div');
    card.className = 'bg-primary border border-black-300 rounded overflow-hidden shadow hover:border-black-100 hover:-translate-y-1 transition-all duration-200 flex flex-col justify-between p-3 relative group cursor-pointer';
    card.style.width = '100%';
    
    const bookIds = p.itemIds || [];
    const count = bookIds.length;

    card.setAttribute('tabindex', '0');
    card.setAttribute('role', 'button');
    card.setAttribute('aria-label', `Playlist: ${p.name || 'Untitled'}, ${count} item${count !== 1 ? 's' : ''}`);
    
    card.innerHTML = `
      <div class="space-y-2">
        <div class="w-full bg-black-500 rounded overflow-hidden flex items-center justify-center text-accent/80 border border-black-400 mb-2 relative" style="aspect-ratio: 1/1;">
          ${renderCoversPreview(bookIds, token)}
          <span class="absolute bottom-2 right-2 bg-black-600/80 px-2 py-0.5 text-[10px] text-white rounded font-semibold z-10">${count} items</span>
        </div>
        <h3 class="font-semibold text-white truncate text-xs" title="${escapeHtml(p.name)}">${escapeHtml(p.name)}</h3>
        <p class="text-[10px] text-black-50 line-clamp-2 h-7 leading-relaxed">${escapeHtml(p.description) || 'No description'}</p>
      </div>

      <div class="flex justify-between items-center mt-3 pt-2 border-t border-black-400/50">
        <button class="delete-btn text-error hover:text-red-400 text-[10px] flex items-center space-x-0.5" data-id="${p.id}">
          <span class="material-symbols text-[13px]">delete</span>
          <span>Delete</span>
        </button>
        <span class="text-[10px] text-accent font-semibold flex items-center space-x-0.5">
          <span>Details</span>
          <span class="material-symbols text-[12px]">chevron_right</span>
        </span>
      </div>
    `;

    card.onclick = (e) => {
      if (e.target.closest('.delete-btn')) return;
      if (window.navigateTo) {
        window.navigateTo('/playlist/' + p.id);
      } else {
        loadPlaylistDetails(p.id, libraryId);
      }
    };

    card.addEventListener('keydown', (e) => {
      if (e.target.closest('.delete-btn')) return;
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        card.click();
      }
    });

    card.querySelector('.delete-btn').onclick = async (e) => {
      e.stopPropagation();
      const confirmed = await window.showConfirm(
        'Delete Playlist',
        `Are you sure you want to delete playlist "${p.name}"?`,
        'Delete',
        'Cancel'
      );
      if (!confirmed) return;
      try {
        await request('DELETE', `/api/playlists/${p.id}`);
        loadPlaylists(libraryId);
      } catch (err) {
        showToast('Failed to delete playlist: ' + err.message, 'error');
      }
    };

    grid.appendChild(card);
  });
}

async function triggerCreatePlaylistModal(libraryId) {
  // Fetch available library items to show in the dropdown/picker
  let items = [];
  try {
    const res = await request('GET', `/api/libraries/${libraryId}/items?limit=500&minified=1`);
    items = res.results || [];
  } catch (err) {
    console.error('Failed to load library items:', err);
  }

  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4';
  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-md p-6 rounded-md shadow-lg space-y-4">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2">Create Playlist</h3>
      
      <div class="space-y-3">
        <div>
          <label class="block text-xs text-black-100 mb-1">Playlist Name</label>
          <input type="text" id="new-playlist-name" required placeholder="My Awesome Playlist" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>
        
        <div>
          <label class="block text-xs text-black-100 mb-1">Add Library Items</label>
          <div class="max-h-48 overflow-y-auto border border-black-300 rounded p-2 bg-black-500 space-y-1.5" id="playlist-item-selector">
            ${items.length === 0 ? '<p class="text-xs text-black-100">No items available</p>' : items.map(item => `
              <label class="flex items-center space-x-2 text-xs cursor-pointer hover:bg-black-400 p-1 rounded">
                <input type="checkbox" value="${item.id}" class="playlist-item-checkbox rounded text-accent bg-black-600 border-black-300">
                <span class="truncate">${escapeHtml(item.media?.metadata?.title || item.title || 'Untitled')}</span>
              </label>
            `).join('')}
          </div>
        </div>
      </div>

      <div class="flex justify-end space-x-3 pt-2">
        <button id="close-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs">Cancel</button>
        <button id="save-playlist-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">Create</button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  modal.querySelector('#close-modal-btn').onclick = closeModal;

  modal.querySelector('#save-playlist-btn').onclick = async () => {
    const name = document.getElementById('new-playlist-name').value.trim();
    if (!name) {
      showToast('Playlist name is required', 'warning');
      return;
    }

    const checkboxes = modal.querySelectorAll('.playlist-item-checkbox:checked');
    const selectedIds = Array.from(checkboxes).map(cb => cb.value);

    try {
      await request('POST', '/api/playlists', {
        name,
        items: selectedIds
      });
      closeModal();
      loadPlaylists(libraryId);
    } catch (err) {
      showToast('Failed to create playlist: ' + err.message, 'error');
    }
  };
}

export async function loadPlaylistDetails(playlistId, libraryId) {
  const container = document.getElementById('bookshelf');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent mx-auto mt-20"></div>`;

  try {
    const playlist = await request('GET', `/api/playlists/${playlistId}`);
    const user = await request('GET', '/api/me').catch(() => null);
    const isAdmin = user && (user.type === 'root' || user.type === 'admin');
    
    // Fetch individual item details to render their titles/covers
    const itemIds = playlist.itemIds || [];
    const itemsDetails = [];

    if (itemIds.length > 0) {
      const detailsPromises = itemIds.map(id => request('GET', `/api/items/${id}`).catch(err => {
        console.warn(`Failed to fetch metadata for item ${id}:`, err);
        return null;
      }));
      const resolved = await Promise.all(detailsPromises);
      resolved.forEach(it => {
        if (it) itemsDetails.push(it);
      });
    }

    container.innerHTML = `
      <div class="p-6 space-y-6 max-w-3xl mx-auto">
        <div class="flex items-center space-x-2">
          <button id="back-playlists-btn" class="flex items-center space-x-1.5 text-sm text-black-100 hover:text-white transition-colors cursor-pointer">
            <span class="material-symbols text-sm">arrow_back</span>
            <span>Back to Playlists</span>
          </button>
        </div>

        <div class="bg-primary border border-black-300 p-6 rounded-md space-y-4">
          <div class="flex flex-col sm:flex-row justify-between items-start gap-4 sm:gap-0 border-b border-black-400 pb-4">
            <div>
              <h2 class="text-2xl font-bold text-white mb-1" id="playlist-title">${escapeHtml(playlist.name)}</h2>
              <p class="text-xs text-black-100">Created: ${window.formatDateTime ? window.formatDateTime(playlist.createdAt) : new Date(playlist.createdAt).toLocaleString()}</p>
            </div>
            <div class="space-x-2 flex items-center w-full sm:w-auto justify-end sm:justify-start flex-shrink-0">
              ${itemsDetails.length > 0 ? `
              <button id="play-playlist-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-3 py-1.5 rounded text-xs inline-flex items-center space-x-1 transition-opacity cursor-pointer">
                <span class="material-symbols text-sm">play_arrow</span>
                <span>Play</span>
              </button>
              ` : ''}
              <button id="edit-playlist-btn" class="bg-black-400 hover:bg-black-300 border border-black-300 text-white font-semibold px-3 py-1.5 rounded text-xs flex items-center space-x-1 transition-colors cursor-pointer">
                <span class="material-symbols text-xs">edit</span>
                <span>Edit Name</span>
              </button>
              <button id="delete-playlist-btn" class="bg-black-400 hover:bg-red-900/40 border border-red-500/30 text-error hover:text-white hover:border-red-500/50 font-semibold px-3 py-1.5 rounded text-xs flex items-center space-x-1 transition-colors cursor-pointer">
                <span class="material-symbols text-xs">delete</span>
                <span>Delete</span>
              </button>
            </div>
          </div>

          <h3 class="font-semibold text-sm text-black-100">Tracks in Playlist (${itemsDetails.length})</h3>
          
          <ul id="playlist-items-list" class="space-y-2">
            <!-- Playlist Items Rows -->
          </ul>

          <div id="playlist-empty-tracks" class="text-center py-10 border border-dashed border-black-400 rounded-md text-black-100 hidden">
            No items in this playlist.
          </div>

          <!-- RSS Feed Status & Management Section -->
          <div id="playlist-rss-section" class="border-t border-black-400/50 pt-4 space-y-3">
            <div class="flex items-center justify-between">
              <h4 class="font-bold text-sm text-white uppercase tracking-wider">RSS Feed</h4>
              <span id="rss-status-badge" class="px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-black-500 text-black-100">Closed</span>
            </div>
            <div id="rss-controls" class="space-y-2 max-w-md">
              <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-accent mx-auto"></div>
            </div>
          </div>
        </div>
      </div>
    `;

    renderPlaylistItemsRows(playlist, itemsDetails, libraryId);

    const playBtn = document.getElementById('play-playlist-btn');
    if (playBtn) {
      playBtn.onclick = async () => {
        await playItems(itemsDetails);
      };
    }

    document.getElementById('back-playlists-btn').onclick = () => {
      if (window.navigateTo) {
        window.navigateTo('/playlists');
      } else {
        loadPlaylists(libraryId);
      }
    };
    
    document.getElementById('delete-playlist-btn').onclick = async () => {
      const confirmed = await window.showConfirm(
        'Delete Playlist',
        `Are you sure you want to delete playlist "${playlist.name}"?`,
        'Delete',
        'Cancel'
      );
      if (!confirmed) return;
      try {
        await request('DELETE', `/api/playlists/${playlist.id}`);
        if (window.navigateTo) {
          window.navigateTo('/playlists');
        } else {
          loadPlaylists(libraryId);
        }
      } catch (err) {
        showToast('Delete failed: ' + err.message, 'error');
      }
    };

    document.getElementById('edit-playlist-btn').onclick = async () => {
      const newName = await window.showPrompt('Rename Playlist', 'Enter new name for playlist:', playlist.name);
      if (newName && newName.trim()) {
        try {
          await request('PATCH', `/api/playlists/${playlist.id}`, { name: newName.trim() });
          loadPlaylistDetails(playlist.id, libraryId);
        } catch (err) {
          showToast('Rename failed: ' + err.message, 'error');
        }
      }
    };

    // RSS Feed Rendering & Logic
    const rssStatusBadge = document.getElementById('rss-status-badge');
    const rssControls = document.getElementById('rss-controls');
    
    async function updateRssSection() {
      if (!rssStatusBadge || !rssControls) return;
      try {
        const feedsResp = await request('GET', '/api/feeds');
        const feeds = feedsResp.feeds || [];
        const activeFeed = feeds.find(f => f.entityId === playlist.id);
        
        if (activeFeed) {
          rssStatusBadge.className = 'px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-accent/20 text-accent';
          rssStatusBadge.textContent = 'Active';
          
          rssControls.innerHTML = `
            <div class="space-y-1.5 w-full">
              <label class="text-[0.65rem] text-black-100 font-semibold uppercase">Feed URL</label>
              <div class="flex gap-1.5 w-full">
                <input type="text" id="rss-feed-url-input" readonly value="${escapeHtml(activeFeed.feedUrl)}" class="flex-grow bg-black-500 text-white font-mono text-[0.7rem] px-2.5 py-1.5 rounded border border-black-300 focus:outline-none select-all w-0">
                <button id="rss-copy-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-3 rounded transition-colors text-[0.7rem] whitespace-nowrap">
                  Copy
                </button>
              </div>
            </div>
            ${isAdmin ? `
              <button id="rss-action-btn" class="w-full bg-error hover:opacity-90 text-white font-bold py-1.5 px-3 rounded transition-all text-[0.7rem]">
                Close RSS Feed
              </button>
            ` : ''}
          `;
          
          const copyBtn = document.getElementById('rss-copy-btn');
          if (copyBtn) {
            copyBtn.onclick = () => {
              const urlInput = document.getElementById('rss-feed-url-input');
              navigator.clipboard.writeText(urlInput ? urlInput.value : activeFeed.feedUrl).then(() => {
                const oldText = copyBtn.textContent;
                copyBtn.textContent = 'Copied';
                setTimeout(() => { copyBtn.textContent = oldText; }, 2000);
              });
            };
          }
          
          const actionBtn = document.getElementById('rss-action-btn');
          if (actionBtn) {
            actionBtn.onclick = async () => {
              const confirmed = await window.showConfirm(
                'Close RSS Feed',
                'Are you sure you want to close this RSS feed?',
                'Close',
                'Cancel'
              );
              if (!confirmed) return;
              try {
                await request('DELETE', `/api/feeds/${activeFeed.id}`);
                updateRssSection();
              } catch (err) {
                showToast('Failed to close RSS feed: ' + err.message, 'error');
              }
            };
          }
        } else {
          rssStatusBadge.className = 'px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-black-500 text-black-100';
          rssStatusBadge.textContent = 'Closed';
          
          if (isAdmin) {
            rssControls.innerHTML = `
              <p class="text-black-100 text-[0.7rem]">Generate a public RSS feed to subscribe to this playlist in external podcast players.</p>
              <button id="rss-action-btn" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-1.5 px-3 rounded transition-all text-[0.7rem]">
                Open Public RSS Feed
              </button>
            `;
            
            const actionBtn = document.getElementById('rss-action-btn');
            if (actionBtn) {
              actionBtn.onclick = async () => {
                try {
                  await request('POST', '/api/feeds', {
                    entityId: playlist.id,
                    type: 'playlist'
                  });
                  updateRssSection();
                } catch (err) {
                  showToast('Failed to open RSS feed: ' + err.message, 'error');
                }
              };
            }
          } else {
            rssControls.innerHTML = `
              <p class="text-black-100 text-[0.7rem]">No active RSS feed. Public RSS feeds must be enabled by an administrator.</p>
            `;
          }
        }
      } catch (err) {
        rssControls.innerHTML = `<p class="text-error text-[0.75rem]">Failed to query RSS details: ${escapeHtml(err.message)}</p>`;
      }
    }
    
    updateRssSection();

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm p-6">Failed to load playlist: ${err.message}</p>`;
  }
}

function renderPlaylistItemsRows(playlist, itemsDetails, libraryId) {
  const list = document.getElementById('playlist-items-list');
  const emptyState = document.getElementById('playlist-empty-tracks');
  if (!list) return;

  list.innerHTML = '';
  if (itemsDetails.length === 0) {
    emptyState.classList.remove('hidden');
    return;
  }
  emptyState.classList.add('hidden');

  let draggedIndex = null;

  itemsDetails.forEach((item, index) => {
    const li = document.createElement('li');
    li.className = 'flex items-center justify-between bg-black-500/40 hover:bg-black-500/80 p-3 rounded border border-black-400/50 transition-colors cursor-move';
    li.draggable = true;

    const token = localStorage.getItem('token');
    const title = item.media?.metadata?.title || item.title || 'Untitled';
    const subtitle = item.mediaType === 'book' ? item.media?.metadata?.authorName || 'Unknown Author' : item.media?.metadata?.author || 'Unknown Author';

    li.innerHTML = `
      <div class="flex items-center space-x-2 flex-grow min-w-0">
        <!-- Drag Handle -->
        <span class="material-symbols text-black-200 hover:text-white text-xl cursor-grab select-none mr-1 drag-handle">drag_handle</span>
        <div class="flex items-center space-x-3 flex-grow cursor-pointer play-trigger min-w-0">
          <img src="${resolvePath(`/api/items/${item.id}/cover?token=${token}&width=80`)}" class="h-12 w-12 object-cover rounded shadow flex-shrink-0" alt="Cover">
          <div class="truncate">
            <p class="font-semibold text-white text-sm truncate">${escapeHtml(title)}</p>
            <p class="text-xs text-black-50 truncate">${escapeHtml(subtitle)}</p>
          </div>
        </div>
      </div>

      <div class="flex items-center space-x-3 ml-4 flex-shrink-0">
        <button class="play-track-btn text-accent hover:opacity-80 p-1" title="Play starting from this track">
          <span class="material-symbols text-lg">play_arrow</span>
        </button>
        <button class="remove-btn text-error hover:text-red-400 p-1" title="Remove from playlist">
          <span class="material-symbols text-lg">close</span>
        </button>
      </div>
    `;

    // Click cover/title views item details
    li.querySelector('.play-trigger').onclick = () => {
      if (window.navigateTo) {
        window.navigateTo('/item/' + item.id);
      } else {
        loadItemDetails(item.id, libraryId, () => loadPlaylistDetails(playlist.id, libraryId));
      }
    };

    li.querySelector('.play-track-btn').onclick = async (e) => {
      e.stopPropagation();
      await playItems(itemsDetails, index);
    };

    // Attach HTML5 drag and drop events
    li.addEventListener('dragstart', (e) => {
      draggedIndex = index;
      li.classList.add('opacity-40');
      e.dataTransfer.effectAllowed = 'move';
    });

    li.addEventListener('dragend', () => {
      li.classList.remove('opacity-40');
      draggedIndex = null;
    });

    li.addEventListener('dragover', (e) => {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
    });

    li.addEventListener('dragenter', () => {
      if (draggedIndex !== null && index !== draggedIndex) {
        li.classList.add('bg-black-400/80');
      }
    });

    li.addEventListener('dragleave', () => {
      li.classList.remove('bg-black-400/80');
    });

    li.addEventListener('drop', async (e) => {
      e.preventDefault();
      li.classList.remove('bg-black-400/80');
      if (draggedIndex !== null && draggedIndex !== index) {
        const newOrderIds = [...playlist.itemIds];
        const element = newOrderIds.splice(draggedIndex, 1)[0];
        newOrderIds.splice(index, 0, element);

        try {
          await request('PATCH', `/api/playlists/${playlist.id}`, { items: newOrderIds });
          loadPlaylistDetails(playlist.id, libraryId);
        } catch (err) {
          showToast('Failed to reorder: ' + err.message, 'error');
        }
      }
    });

    li.querySelector('.remove-btn').onclick = async (e) => {
      e.stopPropagation();
      const confirmed = await window.showConfirm(
        'Remove from Playlist',
        `Remove "${title}" from playlist?`,
        'Remove',
        'Cancel'
      );
      if (!confirmed) return;
      const newOrderIds = playlist.itemIds.filter(id => id !== item.id);
      try {
        await request('PATCH', `/api/playlists/${playlist.id}`, { items: newOrderIds });
        loadPlaylistDetails(playlist.id, libraryId);
      } catch (err) {
        showToast('Failed to remove item: ' + err.message, 'error');
      }
    };

    list.appendChild(li);
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
