import { request, resolvePath } from './api.js';
import { showToast } from './toast.js';
import { playItem } from './player.js';
import { loadItemDetails } from './itemDetails.js';

let currentSearch = '';
let currentSort = 'name'; // 'name', 'numBooks'
let currentDesc = false;
let allCollections = [];

export async function loadCollections(libraryId) {
  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Collections';
  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = 'Loading...';

  container.innerHTML = `<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent mx-auto mt-20"></div>`;

  try {
    const res = await request('GET', `/api/libraries/${libraryId}/collections`);
    allCollections = res.results || [];

    container.innerHTML = `
      <div class="p-6 space-y-6 text-left">
        <div class="flex justify-between items-center">
          <h2 class="text-xl font-bold">Your Collections</h2>
          <button id="create-collection-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity flex items-center space-x-1.5 text-sm">
            <span class="material-symbols text-lg">add</span>
            <span>Create Collection</span>
          </button>
        </div>

        <!-- Controls Toolbar -->
        <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-4 bg-black-600/40 p-4 rounded-lg border border-black-600/30 animate-fade-in">
          <!-- Search input -->
          <div class="relative flex-grow max-w-md">
            <span class="material-symbols absolute left-3 top-2.5 text-black-200 text-lg">search</span>
            <input type="text" id="collections-search" placeholder="Search collections..." value="${escapeHtml(currentSearch)}"
              class="w-full bg-black-500 text-white pl-10 pr-10 py-2 rounded-lg border border-black-300 focus:outline-none focus:border-accent text-sm transition-colors">
            ${currentSearch ? `
              <button id="collections-search-clear-btn" class="absolute right-3 top-2.5 text-black-200 hover:text-white transition-colors focus:outline-none" title="Clear Search">
                <span class="material-symbols text-lg">close</span>
              </button>
            ` : ''}
          </div>

          <!-- Sort and Order controls -->
          <div class="flex items-center gap-3">
            <label class="text-xs font-semibold text-black-100 uppercase tracking-wider">Sort by:</label>
            <select id="collections-sort-select" class="bg-black-500 border border-black-300 text-white text-xs rounded px-3 py-1.5 focus:outline-none cursor-pointer">
              <option value="name" ${currentSort === 'name' ? 'selected' : ''}>Name</option>
              <option value="numBooks" ${currentSort === 'numBooks' ? 'selected' : ''}>Book Count</option>
            </select>
            <button id="collections-direction-btn" class="p-1.5 bg-black-500 hover:bg-black-400 border border-black-300 rounded text-white flex items-center justify-center transition-colors" title="Toggle Sort Order">
              <span class="material-symbols text-lg">${currentDesc ? 'arrow_downward' : 'arrow_upward'}</span>
            </button>
          </div>
        </div>

        <div id="collections-grid" class="library-grid w-full">
          <!-- Collection Cards -->
        </div>

        <div id="collections-empty" class="text-center py-20 bg-primary border border-black-400 rounded-md hidden">
          <span class="material-symbols text-5xl text-black-100 mb-2">collections_bookmark</span>
          <p class="text-black-50 mb-4">No collections found. Group related audiobooks/series into custom collections!</p>
          <button id="create-first-collection-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity text-sm">Create Collection</button>
        </div>
      </div>
    `;

    const updateGrid = () => {
      let filtered = [...allCollections];

      if (currentSearch) {
        const searchLower = currentSearch.toLowerCase();
        filtered = filtered.filter(c =>
          (c.name && c.name.toLowerCase().includes(searchLower)) ||
          (c.description && c.description.toLowerCase().includes(searchLower))
        );
      }

      filtered.sort((a, b) => {
        let comparison = 0;
        if (currentSort === 'numBooks') {
          const countA = (a.books || a.itemIds || []).length;
          const countB = (b.books || b.itemIds || []).length;
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
        bookCount.textContent = filtered.length === 1 ? '1 Collection' : `${filtered.length} Collections`;
      }

      renderCollectionsGrid(filtered, libraryId);

      // Wire up the empty state button if needed
      const emptyBtn = document.getElementById('create-first-collection-btn');
      if (emptyBtn) emptyBtn.onclick = showCreateModal;
    };

    const showCreateModal = () => triggerCreateCollectionModal(libraryId);
    document.getElementById('create-collection-btn').onclick = showCreateModal;

    // Listeners
    const searchInput = document.getElementById('collections-search');
    if (searchInput) {
      searchInput.addEventListener('input', (e) => {
        currentSearch = e.target.value;
        loadCollections(libraryId); // reload structure to reflect show/hide clear btn
      });
    }

    const clearSearchBtn = document.getElementById('collections-search-clear-btn');
    if (clearSearchBtn) {
      clearSearchBtn.addEventListener('click', () => {
        currentSearch = '';
        loadCollections(libraryId);
      });
    }

    const sortSelect = document.getElementById('collections-sort-select');
    if (sortSelect) {
      sortSelect.addEventListener('change', (e) => {
        currentSort = e.target.value;
        updateGrid();
      });
    }

    const directionBtn = document.getElementById('collections-direction-btn');
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
    container.innerHTML = `<p class="text-red-400 text-sm p-6">Failed to load collections: ${err.message}</p>`;
  }
}

function renderCoversPreview(bookIds, token) {
  if (!bookIds || bookIds.length === 0) {
    return `<span class="material-symbols text-4xl text-accent/80">collections_bookmark</span>`;
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

function renderCollectionsGrid(collections, libraryId) {
  const grid = document.getElementById('collections-grid');
  const emptyState = document.getElementById('collections-empty');
  if (!grid) return;

  grid.innerHTML = '';
  if (collections.length === 0) {
    emptyState.classList.remove('hidden');
    return;
  }
  emptyState.classList.add('hidden');

  const token = localStorage.getItem('token');

  collections.forEach(c => {
    const card = document.createElement('div');
    card.className = 'bg-primary border border-black-300 rounded overflow-hidden shadow hover:border-black-100 hover:-translate-y-1 transition-all duration-200 flex flex-col justify-between p-3 relative group cursor-pointer';
    card.style.width = '100%';
    
    const bookIds = c.books || c.itemIds || [];
    const count = bookIds.length;

    card.setAttribute('tabindex', '0');
    card.setAttribute('role', 'button');
    card.setAttribute('aria-label', `${c.isSmart ? 'Smart ' : ''}Collection: ${c.name || 'Untitled'}, ${count} book${count !== 1 ? 's' : ''}`);
    
    card.innerHTML = `
      <div class="space-y-2">
        <div class="w-full bg-black-500 rounded overflow-hidden flex items-center justify-center text-accent/80 border border-black-400 mb-2 relative" style="aspect-ratio: 1/1;">
          ${c.isSmart ? `<span class="absolute top-2 left-2 bg-accent/20 text-accent border border-accent/30 px-1.5 py-0.5 text-[0.65rem] rounded font-bold uppercase tracking-wide z-10">Smart</span>` : ''}
          ${renderCoversPreview(bookIds, token)}
          <span class="absolute bottom-2 right-2 bg-black-600/80 px-2 py-0.5 text-[10px] text-white rounded font-semibold z-10">${count} books</span>
        </div>
        <h3 class="font-semibold text-white truncate text-xs" title="${escapeHtml(c.name)}">${escapeHtml(c.name)}</h3>
        <p class="text-[10px] text-black-50 line-clamp-2 h-7 leading-relaxed">${escapeHtml(c.description) || 'No description'}</p>
      </div>

      <div class="flex justify-between items-center mt-3 pt-2 border-t border-black-400/50">
        <button class="delete-btn text-error hover:text-red-400 text-[10px] flex items-center space-x-0.5" data-id="${c.id}">
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
        window.navigateTo('/collection/' + c.id);
      } else {
        loadCollectionDetails(c.id, libraryId);
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
        'Delete Collection',
        `Are you sure you want to delete collection "${c.name}"?`,
        'Delete',
        'Cancel'
      );
      if (!confirmed) return;
      try {
        await request('DELETE', `/api/collections/${c.id}`);
        loadCollections(libraryId);
      } catch (err) {
        showToast('Failed to delete collection: ' + err.message, 'error');
      }
    };

    grid.appendChild(card);
  });
}

async function triggerCreateCollectionModal(libraryId) {
  // Fetch available library items to show in the selector
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
      <h3 class="text-lg font-bold border-b border-black-400 pb-2">Create Collection</h3>
      
      <div class="space-y-3">
        <div>
          <label class="block text-xs text-black-100 mb-1">Collection Name</label>
          <input type="text" id="new-coll-name" required placeholder="My Favorite Books" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">Description</label>
          <textarea id="new-coll-desc" placeholder="A short description of this collection" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm h-16 resize-none"></textarea>
        </div>

        <div>
          <label class="flex items-center space-x-2 text-xs font-semibold text-white cursor-pointer my-2">
            <input type="checkbox" id="new-coll-smart-toggle" class="rounded text-accent bg-black-600 border-black-300">
            <span>Smart Collection (rules-based)</span>
          </label>
        </div>
        
        <div id="new-coll-manual-books-container">
          <label class="block text-xs text-black-100 mb-1">Select Books</label>
          <div class="max-h-48 overflow-y-auto border border-black-300 rounded p-2 bg-black-500 space-y-1.5" id="coll-book-selector">
            ${items.length === 0 ? '<p class="text-xs text-black-100">No items available</p>' : items.map(item => `
              <label class="flex items-center space-x-2 text-xs cursor-pointer hover:bg-black-400 p-1 rounded">
                <input type="checkbox" value="${item.id}" class="coll-book-checkbox rounded text-accent bg-black-600 border-black-300">
                <span class="truncate">${escapeHtml(item.media?.metadata?.title || item.title || 'Untitled')}</span>
              </label>
            `).join('')}
          </div>
        </div>

        <div id="new-coll-smart-rules-container" class="hidden space-y-3 border border-black-300 rounded p-3 bg-black-500/50">
          <p class="text-[0.7rem] text-accent/80 flex items-center space-x-1 mb-2 font-semibold">
            <span class="material-symbols text-xs">info</span>
            <span>Books matching any of the rules below will be automatically added.</span>
          </p>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Genres (comma separated)</label>
            <input type="text" id="new-coll-rules-genres" placeholder="e.g. Fantasy, Sci-Fi" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Tags (comma separated)</label>
            <input type="text" id="new-coll-rules-tags" placeholder="e.g. Favorite, Unfinished" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Narrators (comma separated)</label>
            <input type="text" id="new-coll-rules-narrators" placeholder="e.g. Stephen Fry" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Authors (comma separated)</label>
            <input type="text" id="new-coll-rules-authors" placeholder="e.g. J.K. Rowling, J.R.R. Tolkien" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Published Years (comma separated)</label>
            <input type="text" id="new-coll-rules-published-years" placeholder="e.g. 1997, 2001" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Search Query</label>
            <input type="text" id="new-coll-rules-search" placeholder="e.g. Harry Potter" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>
      </div>

      <div class="flex justify-end space-x-3 pt-2">
        <button id="close-coll-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs">Cancel</button>
        <button id="save-coll-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">Create</button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const smartToggle = modal.querySelector('#new-coll-smart-toggle');
  const manualContainer = modal.querySelector('#new-coll-manual-books-container');
  const smartContainer = modal.querySelector('#new-coll-smart-rules-container');

  smartToggle.onchange = (e) => {
    if (e.target.checked) {
      manualContainer.classList.add('hidden');
      smartContainer.classList.remove('hidden');
    } else {
      manualContainer.classList.remove('hidden');
      smartContainer.classList.add('hidden');
    }
  };

  const closeModal = () => modal.remove();
  modal.querySelector('#close-coll-modal-btn').onclick = closeModal;

  modal.querySelector('#save-coll-btn').onclick = async () => {
    const name = document.getElementById('new-coll-name').value.trim();
    const description = document.getElementById('new-coll-desc').value.trim();
    if (!name) {
      showToast('Collection name is required', 'warning');
      return;
    }

    const isSmart = smartToggle.checked;
    let books = [];
    let rulesStr = "";

    if (isSmart) {
      const genres = document.getElementById('new-coll-rules-genres').value.split(',').map(s => s.trim()).filter(Boolean);
      const tags = document.getElementById('new-coll-rules-tags').value.split(',').map(s => s.trim()).filter(Boolean);
      const narrators = document.getElementById('new-coll-rules-narrators').value.split(',').map(s => s.trim()).filter(Boolean);
      const authors = document.getElementById('new-coll-rules-authors').value.split(',').map(s => s.trim()).filter(Boolean);
      const publishedYears = document.getElementById('new-coll-rules-published-years').value.split(',').map(s => s.trim()).filter(Boolean);
      const search = document.getElementById('new-coll-rules-search').value.trim();
      rulesStr = JSON.stringify({ genres, tags, narrators, authors, publishedYears, search });
    } else {
      const checkboxes = modal.querySelectorAll('.coll-book-checkbox:checked');
      books = Array.from(checkboxes).map(cb => cb.value);
    }

    try {
      await request('POST', '/api/collections', {
        name,
        description,
        libraryId,
        isSmart,
        rules: rulesStr,
        books
      });
      closeModal();
      loadCollections(libraryId);
    } catch (err) {
      showToast('Failed to create collection: ' + err.message, 'error');
    }
  };
}

export async function loadCollectionDetails(collectionId, libraryId) {
  const container = document.getElementById('bookshelf');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent mx-auto mt-20"></div>`;

  try {
    const collection = await request('GET', `/api/collections/${collectionId}`);
    
    // Fetch individual item details to render their titles/covers
    const bookIds = collection.books || collection.itemIds || [];
    const booksDetails = [];

    if (bookIds.length > 0) {
      const detailsPromises = bookIds.map(id => request('GET', `/api/items/${id}`).catch(err => {
        console.warn(`Failed to fetch metadata for item ${id}:`, err);
        return null;
      }));
      const resolved = await Promise.all(detailsPromises);
      resolved.forEach(it => {
        if (it) booksDetails.push(it);
      });
    }

    container.innerHTML = `
      <div class="p-6 space-y-6 max-w-3xl mx-auto">
        <div class="flex items-center space-x-2">
          <button id="back-collections-btn" class="flex items-center space-x-1 text-sm text-black-50 hover:text-white">
            <span class="material-symbols">arrow_back</span>
            <span>Back to Collections</span>
          </button>
        </div>

        <div class="bg-primary border border-black-300 p-6 rounded-md space-y-4">
          <div class="flex flex-col sm:flex-row justify-between items-start gap-4 sm:gap-0 border-b border-black-400 pb-4">
            <div>
              <div class="flex items-center space-x-2 mb-1">
                <h2 class="text-2xl font-bold text-white" id="coll-details-title">${escapeHtml(collection.name)}</h2>
                ${collection.isSmart ? `<span class="bg-accent/20 text-accent border border-accent/30 px-2 py-0.5 text-xs rounded font-bold uppercase tracking-wide">Smart</span>` : ''}
              </div>
              <p class="text-xs text-black-50 mb-2" id="coll-details-desc">${escapeHtml(collection.description) || 'No description provided.'}</p>
              <p class="text-xs text-black-100">Created: ${window.formatDateTime ? window.formatDateTime(collection.createdAt) : new Date(collection.createdAt).toLocaleString()}</p>
            </div>
            <div class="space-x-2 flex-shrink-0 flex items-center w-full sm:w-auto justify-end sm:justify-start">
              <button id="edit-coll-btn" class="bg-black-400 hover:bg-black-300 border border-black-300 text-white font-semibold px-3 py-1.5 rounded text-xs">Edit Details</button>
              <button id="delete-coll-btn" class="bg-red-900 hover:bg-red-800 text-red-200 font-semibold px-3 py-1.5 rounded text-xs">Delete</button>
            </div>
          </div>

          <h3 class="font-semibold text-sm text-black-100">Books in Collection (${booksDetails.length})</h3>
          
          <ul id="coll-books-list" class="space-y-2">
            <!-- Collection Books Rows -->
          </ul>

          <div id="coll-empty-tracks" class="text-center py-10 border border-dashed border-black-400 rounded-md text-black-100 hidden">
            No books in this collection.
          </div>

          <!-- RSS Feed Status & Management Section -->
          <div id="collection-rss-section" class="border-t border-black-400/50 pt-4 space-y-3">
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

    renderCollectionBooksRows(collection, booksDetails, libraryId);

    document.getElementById('back-collections-btn').onclick = () => {
      if (window.navigateTo) {
        window.navigateTo('/collections');
      } else {
        loadCollections(libraryId);
      }
    };
    
    document.getElementById('delete-coll-btn').onclick = async () => {
      const confirmed = await window.showConfirm(
        'Delete Collection',
        `Are you absolutely sure you want to delete collection "${collection.name}"?`,
        'Delete',
        'Cancel'
      );
      if (!confirmed) return;
      try {
        await request('DELETE', `/api/collections/${collection.id}`);
        if (window.navigateTo) {
          window.navigateTo('/collections');
        } else {
          loadCollections(libraryId);
        }
      } catch (err) {
        showToast('Delete failed: ' + err.message, 'error');
      }
    };

    document.getElementById('edit-coll-btn').onclick = () => triggerEditCollectionDetailsModal(collection, libraryId);

    // Fetch user and render RSS Feed management section
    const user = await request('GET', '/api/me').catch(() => null);
    const isAdmin = user && (user.type === 'root' || user.type === 'admin');

    const rssStatusBadge = document.getElementById('rss-status-badge');
    const rssControls = document.getElementById('rss-controls');
    
    async function updateRssSection() {
      if (!rssStatusBadge || !rssControls) return;
      try {
        const feedsResp = await request('GET', '/api/feeds');
        const feeds = feedsResp.feeds || [];
        const activeFeed = feeds.find(f => f.entityId === collection.id);
        
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
                showToast('Feed URL copied to clipboard', 'success');
              }).catch(err => {
                showToast('Failed to copy feed URL: ' + err.message, 'error');
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
                showToast('RSS feed closed successfully', 'success');
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
              <p class="text-black-100 text-[0.7rem]">Generate a public RSS feed to subscribe to this collection in external podcast players.</p>
              <button id="rss-action-btn" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-1.5 px-3 rounded transition-all text-[0.7rem]">
                Open Public RSS Feed
              </button>
            `;
            
            const actionBtn = document.getElementById('rss-action-btn');
            if (actionBtn) {
              actionBtn.onclick = async () => {
                try {
                  await request('POST', '/api/feeds', {
                    entityId: collection.id,
                    type: 'collection'
                  });
                  showToast('RSS feed opened successfully', 'success');
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
    container.innerHTML = `<p class="text-red-400 text-sm p-6">Failed to load collection details: ${err.message}</p>`;
  }
}

function renderCollectionBooksRows(collection, booksDetails, libraryId) {
  const list = document.getElementById('coll-books-list');
  const emptyState = document.getElementById('coll-empty-tracks');
  if (!list) return;

  list.innerHTML = '';
  if (booksDetails.length === 0) {
    emptyState.classList.remove('hidden');
    return;
  }
  emptyState.classList.add('hidden');

  const bookIds = collection.books || collection.itemIds || [];
  const isSmart = !!collection.isSmart;
  let draggedIndex = null;

  booksDetails.forEach((item, index) => {
    const li = document.createElement('li');
    if (!isSmart) {
      li.className = 'flex items-center justify-between bg-black-500/40 hover:bg-black-500/80 p-3 rounded border border-black-400/50 transition-colors cursor-move';
      li.draggable = true;
    } else {
      li.className = 'flex items-center justify-between bg-black-500/40 hover:bg-black-500/80 p-3 rounded border border-black-400/50 transition-colors';
    }

    const token = localStorage.getItem('token');
    const title = item.media?.metadata?.title || item.title || 'Untitled';
    const subtitle = item.mediaType === 'book' ? item.media?.metadata?.authorName || 'Unknown Author' : item.media?.metadata?.author || 'Unknown Author';

    li.innerHTML = `
      <div class="flex items-center space-x-2 flex-grow min-w-0">
        ${!isSmart ? `
          <!-- Drag Handle -->
          <span class="material-symbols text-black-200 hover:text-white text-xl cursor-grab select-none mr-1 drag-handle">drag_handle</span>
        ` : ''}
        <div class="flex items-center space-x-3 flex-grow cursor-pointer play-trigger min-w-0">
          <img src="${resolvePath(`/api/items/${item.id}/cover?token=${token}&width=80`)}" class="h-12 w-12 object-cover rounded shadow flex-shrink-0" alt="Cover">
          <div class="truncate">
            <p class="font-semibold text-white text-sm truncate">${escapeHtml(title)}</p>
            <p class="text-xs text-black-50 truncate">${escapeHtml(subtitle)}</p>
          </div>
        </div>
      </div>

      ${!isSmart ? `
      <div class="flex items-center space-x-3 ml-4 flex-shrink-0">
        <!-- Reorder triggers -->
        <button class="up-btn text-black-50 hover:text-white p-1" ${index === 0 ? 'disabled opacity-20' : ''}>
          <span class="material-symbols text-lg">arrow_upward</span>
        </button>
        <button class="down-btn text-black-50 hover:text-white p-1" ${index === booksDetails.length - 1 ? 'disabled opacity-20' : ''}>
          <span class="material-symbols text-lg">arrow_downward</span>
        </button>
        <button class="remove-btn text-error hover:text-red-400 p-1" title="Remove from collection">
          <span class="material-symbols text-lg">close</span>
        </button>
      </div>
      ` : ''}
    `;

    // Click cover/title views item details
    li.querySelector('.play-trigger').onclick = () => {
      if (window.navigateTo) {
        window.navigateTo('/item/' + item.id);
      } else {
        loadItemDetails(item.id, libraryId, () => loadCollectionDetails(collection.id, libraryId));
      }
    };

    if (!isSmart) {
      // Reorder actions
      li.querySelector('.up-btn').onclick = async (e) => {
        e.stopPropagation();
        const newOrderIds = [...bookIds];
        const temp = newOrderIds[index];
        newOrderIds[index] = newOrderIds[index - 1];
        newOrderIds[index - 1] = temp;
        
        try {
          await request('PATCH', `/api/collections/${collection.id}`, { books: newOrderIds });
          loadCollectionDetails(collection.id, libraryId);
        } catch (err) {
          showToast('Failed to reorder collection books: ' + err.message, 'error');
        }
      };

      li.querySelector('.down-btn').onclick = async (e) => {
        e.stopPropagation();
        const newOrderIds = [...bookIds];
        const temp = newOrderIds[index];
        newOrderIds[index] = newOrderIds[index + 1];
        newOrderIds[index + 1] = temp;

        try {
          await request('PATCH', `/api/collections/${collection.id}`, { books: newOrderIds });
          loadCollectionDetails(collection.id, libraryId);
        } catch (err) {
          showToast('Failed to reorder collection books: ' + err.message, 'error');
        }
      };

      li.querySelector('.remove-btn').onclick = async (e) => {
        e.stopPropagation();
        const confirmed = await window.showConfirm(
          'Remove from Collection',
          `Remove "${title}" from collection?`,
          'Remove',
          'Cancel'
        );
        if (!confirmed) return;
        const newOrderIds = bookIds.filter(id => id !== item.id);
        try {
          await request('PATCH', `/api/collections/${collection.id}`, { books: newOrderIds });
          loadCollectionDetails(collection.id, libraryId);
        } catch (err) {
          showToast('Failed to remove book: ' + err.message, 'error');
        }
      };

      // HTML5 Drag & Drop event listeners
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
          const newOrderIds = [...bookIds];
          const element = newOrderIds.splice(draggedIndex, 1)[0];
          newOrderIds.splice(index, 0, element);

          try {
            await request('PATCH', `/api/collections/${collection.id}`, { books: newOrderIds });
            loadCollectionDetails(collection.id, libraryId);
          } catch (err) {
            showToast('Failed to reorder collection books: ' + err.message, 'error');
          }
        }
      });
    }

    list.appendChild(li);
  });
}

async function triggerEditCollectionDetailsModal(collection, libraryId) {
  let items = [];
  try {
    const res = await request('GET', `/api/libraries/${libraryId}/items?limit=500&minified=1`);
    items = res.results || [];
  } catch (err) {
    console.error('Failed to load library items:', err);
  }

  const bookIds = collection.books || collection.itemIds || [];

  let rules = {};
  try {
    rules = typeof collection.rules === 'string' && collection.rules ? JSON.parse(collection.rules) : (collection.rules || {});
  } catch (e) {
    console.error('Failed to parse collection rules:', e);
  }

  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4';
  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-md p-6 rounded-md shadow-lg space-y-4">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2">Edit Collection</h3>
      
      <div class="space-y-3">
        <div>
          <label class="block text-xs text-black-100 mb-1">Collection Name</label>
          <input type="text" id="edit-coll-name" required value="${escapeHtml(collection.name)}" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">Description</label>
          <textarea id="edit-coll-desc" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm h-16 resize-none">${escapeHtml(collection.description || '')}</textarea>
        </div>

        <div>
          <label class="flex items-center space-x-2 text-xs font-semibold text-white cursor-pointer my-2">
            <input type="checkbox" id="edit-coll-smart-toggle" ${collection.isSmart ? 'checked' : ''} class="rounded text-accent bg-black-600 border-black-300">
            <span>Smart Collection (rules-based)</span>
          </label>
        </div>

        <div id="edit-coll-manual-books-container" class="${collection.isSmart ? 'hidden' : ''}">
          <label class="block text-xs text-black-100 mb-1">Select Books</label>
          <div class="max-h-48 overflow-y-auto border border-black-300 rounded p-2 bg-black-500 space-y-1.5" id="edit-coll-book-selector">
            ${items.length === 0 ? '<p class="text-xs text-black-100">No items available</p>' : items.map(item => `
              <label class="flex items-center space-x-2 text-xs cursor-pointer hover:bg-black-400 p-1 rounded">
                <input type="checkbox" value="${item.id}" ${bookIds.includes(item.id) ? 'checked' : ''} class="edit-coll-book-checkbox rounded text-accent bg-black-600 border-black-300">
                <span class="truncate">${escapeHtml(item.media?.metadata?.title || item.title || 'Untitled')}</span>
              </label>
            `).join('')}
          </div>
        </div>

        <div id="edit-coll-smart-rules-container" class="${collection.isSmart ? '' : 'hidden'} space-y-3 border border-black-300 rounded p-3 bg-black-500/50">
          <p class="text-[0.7rem] text-accent/80 flex items-center space-x-1 mb-2 font-semibold">
            <span class="material-symbols text-xs">info</span>
            <span>Books matching any of the rules below will be automatically added.</span>
          </p>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Genres (comma separated)</label>
            <input type="text" id="edit-coll-rules-genres" value="${escapeHtml(rules.genres?.join(', ') || '')}" placeholder="e.g. Fantasy, Sci-Fi" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Tags (comma separated)</label>
            <input type="text" id="edit-coll-rules-tags" value="${escapeHtml(rules.tags?.join(', ') || '')}" placeholder="e.g. Favorite, Unfinished" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Narrators (comma separated)</label>
            <input type="text" id="edit-coll-rules-narrators" value="${escapeHtml(rules.narrators?.join(', ') || '')}" placeholder="e.g. Stephen Fry" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Authors (comma separated)</label>
            <input type="text" id="edit-coll-rules-authors" value="${escapeHtml(rules.authors?.join(', ') || '')}" placeholder="e.g. J.K. Rowling, J.R.R. Tolkien" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Published Years (comma separated)</label>
            <input type="text" id="edit-coll-rules-published-years" value="${escapeHtml(rules.publishedYears?.join(', ') || '')}" placeholder="e.g. 1997, 2001" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.65rem] text-black-100 uppercase tracking-wider mb-1">Search Query</label>
            <input type="text" id="edit-coll-rules-search" value="${escapeHtml(rules.search || '')}" placeholder="e.g. Harry Potter" class="w-full bg-black-500 text-white px-3 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>
      </div>

      <div class="flex justify-end space-x-3 pt-2">
        <button id="close-edit-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs">Cancel</button>
        <button id="save-edit-coll-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">Save</button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const smartToggle = modal.querySelector('#edit-coll-smart-toggle');
  const manualContainer = modal.querySelector('#edit-coll-manual-books-container');
  const smartContainer = modal.querySelector('#edit-coll-smart-rules-container');

  smartToggle.onchange = (e) => {
    if (e.target.checked) {
      manualContainer.classList.add('hidden');
      smartContainer.classList.remove('hidden');
    } else {
      manualContainer.classList.remove('hidden');
      smartContainer.classList.add('hidden');
    }
  };

  const closeModal = () => modal.remove();
  modal.querySelector('#close-edit-modal-btn').onclick = closeModal;

  modal.querySelector('#save-edit-coll-btn').onclick = async () => {
    const name = document.getElementById('edit-coll-name').value.trim();
    const description = document.getElementById('edit-coll-desc').value.trim();
    if (!name) {
      showToast('Collection name is required', 'warning');
      return;
    }

    const isSmart = smartToggle.checked;
    let books = [];
    let rulesStr = "";

    if (isSmart) {
      const genres = document.getElementById('edit-coll-rules-genres').value.split(',').map(s => s.trim()).filter(Boolean);
      const tags = document.getElementById('edit-coll-rules-tags').value.split(',').map(s => s.trim()).filter(Boolean);
      const narrators = document.getElementById('edit-coll-rules-narrators').value.split(',').map(s => s.trim()).filter(Boolean);
      const authors = document.getElementById('edit-coll-rules-authors').value.split(',').map(s => s.trim()).filter(Boolean);
      const publishedYears = document.getElementById('edit-coll-rules-published-years').value.split(',').map(s => s.trim()).filter(Boolean);
      const search = document.getElementById('edit-coll-rules-search').value.trim();
      rulesStr = JSON.stringify({ genres, tags, narrators, authors, publishedYears, search });
    } else {
      const checkboxes = modal.querySelectorAll('.edit-coll-book-checkbox:checked');
      books = Array.from(checkboxes).map(cb => cb.value);
    }

    try {
      await request('PATCH', `/api/collections/${collection.id}`, {
        name,
        description,
        isSmart,
        rules: rulesStr,
        books
      });
      closeModal();
      loadCollectionDetails(collection.id, libraryId);
    } catch (err) {
      showToast('Failed to update collection: ' + err.message, 'error');
    }
  };
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
