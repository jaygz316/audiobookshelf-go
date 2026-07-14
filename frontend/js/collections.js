import { request, resolvePath } from './api.js';
import { playItem } from './player.js';
import { loadItemDetails } from './itemDetails.js';

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
    const collections = res.results || [];

    if (bookCount) bookCount.textContent = `${collections.length} Collections`;

    container.innerHTML = `
      <div class="p-6 space-y-6">
        <div class="flex justify-between items-center">
          <h2 class="text-xl font-bold">Your Collections</h2>
          <button id="create-collection-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity flex items-center space-x-1.5 text-sm">
            <span class="material-symbols text-lg">add</span>
            <span>Create Collection</span>
          </button>
        </div>

        <div id="collections-grid" class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-6">
          <!-- Collection Cards -->
        </div>

        <div id="collections-empty" class="text-center py-20 bg-primary border border-black-400 rounded-md hidden">
          <span class="material-symbols text-5xl text-black-100 mb-2">collections_bookmark</span>
          <p class="text-black-50 mb-4">No collections found. Group related audiobooks/series into custom collections!</p>
          <button id="create-first-collection-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded transition-opacity text-sm">Create Collection</button>
        </div>
      </div>
    `;

    renderCollectionsGrid(collections, libraryId);

    const showCreateModal = () => triggerCreateCollectionModal(libraryId);
    document.getElementById('create-collection-btn').onclick = showCreateModal;
    const emptyBtn = document.getElementById('create-first-collection-btn');
    if (emptyBtn) emptyBtn.onclick = showCreateModal;

  } catch (err) {
    container.innerHTML = `<p class="text-red-400 text-sm p-6">Failed to load collections: ${err.message}</p>`;
  }
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

  collections.forEach(c => {
    const card = document.createElement('div');
    card.className = 'bg-primary border border-black-300 rounded overflow-hidden shadow hover:border-black-100 transition-all flex flex-col justify-between p-4 relative group cursor-pointer';
    
    const count = c.books?.length || 0;
    
    card.innerHTML = `
      <div class="space-y-2">
        <div class="h-28 w-full bg-black-500 rounded flex items-center justify-center text-accent/80 border border-black-400 mb-2 relative">
          ${c.isSmart ? `<span class="absolute top-2 left-2 bg-accent/20 text-accent border border-accent/30 px-1.5 py-0.5 text-[0.65rem] rounded font-bold uppercase tracking-wide">Smart</span>` : ''}
          <span class="material-symbols text-4xl">collections_bookmark</span>
          <span class="absolute bottom-2 right-2 bg-black-600/80 px-2 py-0.5 text-xs text-white rounded font-semibold">${count} books</span>
        </div>
        <h3 class="font-semibold text-white truncate">${escapeHtml(c.name)}</h3>
        <p class="text-xs text-black-50 line-clamp-2">${escapeHtml(c.description) || 'No description'}</p>
      </div>

      <div class="flex justify-between items-center mt-4 pt-2 border-t border-black-400/50">
        <button class="delete-btn text-error hover:text-red-400 text-xs flex items-center space-x-1" data-id="${c.id}">
          <span class="material-symbols text-sm">delete</span>
          <span>Delete</span>
        </button>
        <span class="text-[0.7rem] text-black-100">Click to view</span>
      </div>
    `;

    // Click card navigates to details
    card.onclick = (e) => {
      if (e.target.closest('.delete-btn')) return;
      if (window.navigateTo) {
        window.navigateTo('/collection/' + c.id);
      } else {
        loadCollectionDetails(c.id, libraryId);
      }
    };

    card.querySelector('.delete-btn').onclick = async (e) => {
      e.stopPropagation();
      if (!confirm(`Are you sure you want to delete collection "${c.name}"?`)) return;
      try {
        await request('DELETE', `/api/collections/${c.id}`);
        loadCollections(libraryId);
      } catch (err) {
        alert('Failed to delete collection: ' + err.message);
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
      alert('Collection name is required');
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
      alert('Failed to create collection: ' + err.message);
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
          <div class="flex justify-between items-start border-b border-black-400 pb-4">
            <div>
              <div class="flex items-center space-x-2 mb-1">
                <h2 class="text-2xl font-bold text-white" id="coll-details-title">${escapeHtml(collection.name)}</h2>
                ${collection.isSmart ? `<span class="bg-accent/20 text-accent border border-accent/30 px-2 py-0.5 text-xs rounded font-bold uppercase tracking-wide">Smart</span>` : ''}
              </div>
              <p class="text-xs text-black-50 mb-2" id="coll-details-desc">${escapeHtml(collection.description) || 'No description provided.'}</p>
              <p class="text-xs text-black-100">Created: ${window.formatDateTime ? window.formatDateTime(collection.createdAt) : new Date(collection.createdAt).toLocaleString()}</p>
            </div>
            <div class="space-x-2 flex-shrink-0">
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
      if (!confirm(`Are you absolutely sure you want to delete collection "${collection.name}"?`)) return;
      try {
        await request('DELETE', `/api/collections/${collection.id}`);
        if (window.navigateTo) {
          window.navigateTo('/collections');
        } else {
          loadCollections(libraryId);
        }
      } catch (err) {
        alert('Delete failed: ' + err.message);
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
                const oldText = copyBtn.textContent;
                copyBtn.textContent = 'Copied';
                setTimeout(() => { copyBtn.textContent = oldText; }, 2000);
              });
            };
          }
          
          const actionBtn = document.getElementById('rss-action-btn');
          if (actionBtn) {
            actionBtn.onclick = async () => {
              if (!confirm('Are you sure you want to close this RSS feed?')) return;
              try {
                await request('DELETE', `/api/feeds/${activeFeed.id}`);
                updateRssSection();
              } catch (err) {
                alert('Failed to close RSS feed: ' + err.message);
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
                  updateRssSection();
                } catch (err) {
                  alert('Failed to open RSS feed: ' + err.message);
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

  booksDetails.forEach((item, index) => {
    const li = document.createElement('li');
    li.className = 'flex items-center justify-between bg-black-500/40 hover:bg-black-500/80 p-3 rounded border border-black-400/50 transition-colors';

    const token = localStorage.getItem('token');
    const title = item.media?.metadata?.title || item.title || 'Untitled';
    const subtitle = item.mediaType === 'book' ? item.media?.metadata?.authorName || 'Unknown Author' : item.media?.metadata?.author || 'Unknown Author';

    li.innerHTML = `
      <div class="flex items-center space-x-3 flex-grow cursor-pointer play-trigger">
        <img src="${resolvePath(`/api/items/${item.id}/cover?token=${token}&width=80`)}" class="h-12 w-12 object-cover rounded shadow" alt="Cover">
        <div class="truncate">
          <p class="font-semibold text-white text-sm truncate">${escapeHtml(title)}</p>
          <p class="text-xs text-black-50 truncate">${escapeHtml(subtitle)}</p>
        </div>
      </div>

      ${!isSmart ? `
      <div class="flex items-center space-x-3 ml-4">
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
      li.querySelector('.up-btn').onclick = async () => {
        const newOrderIds = [...bookIds];
        const temp = newOrderIds[index];
        newOrderIds[index] = newOrderIds[index - 1];
        newOrderIds[index - 1] = temp;
        
        try {
          await request('PATCH', `/api/collections/${collection.id}`, { books: newOrderIds });
          loadCollectionDetails(collection.id, libraryId);
        } catch (err) {
          alert('Failed to reorder collection books: ' + err.message);
        }
      };

      li.querySelector('.down-btn').onclick = async () => {
        const newOrderIds = [...bookIds];
        const temp = newOrderIds[index];
        newOrderIds[index] = newOrderIds[index + 1];
        newOrderIds[index + 1] = temp;

        try {
          await request('PATCH', `/api/collections/${collection.id}`, { books: newOrderIds });
          loadCollectionDetails(collection.id, libraryId);
        } catch (err) {
          alert('Failed to reorder collection books: ' + err.message);
        }
      };

      li.querySelector('.remove-btn').onclick = async () => {
        if (!confirm(`Remove "${title}" from collection?`)) return;
        const newOrderIds = bookIds.filter(id => id !== item.id);
        try {
          await request('PATCH', `/api/collections/${collection.id}`, { books: newOrderIds });
          loadCollectionDetails(collection.id, libraryId);
        } catch (err) {
          alert('Failed to remove book: ' + err.message);
        }
      };
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
      alert('Collection name is required');
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
      alert('Failed to update collection: ' + err.message);
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
