// js/dashboard.js

import { request, resolvePath } from './api.js';
import { playItem } from './player.js';
import { loadItemDetails } from './itemDetails.js';
import { showToast } from './app.js';

let batchEditMode = false;
const selectedItems = new Set();

const ALL_COLUMNS = [
  { key: 'cover', label: 'Cover', default: true },
  { key: 'title', label: 'Title', default: true },
  { key: 'author', label: 'Author', default: true },
  { key: 'narrator', label: 'Narrator', default: false },
  { key: 'series', label: 'Series', default: true },
  { key: 'duration', label: 'Duration', default: true },
  { key: 'dateAdded', label: 'Date Added', default: false },
  { key: 'year', label: 'Release Year', default: false },
  { key: 'progress', label: 'Progress', default: false },
  { key: 'action', label: 'Action', default: true }
];

function getVisibleColumns() {
  try {
    const saved = localStorage.getItem('list-view-columns');
    if (saved) return JSON.parse(saved);
  } catch (e) {}
  return ALL_COLUMNS.filter(c => c.default).map(c => c.key);
}

function saveVisibleColumns(columns) {
  localStorage.setItem('list-view-columns', JSON.stringify(columns));
}

export async function loadDashboard(libraryId, filterBy = '', filterLabel = '') {
  const bookshelfContainer = document.getElementById('bookshelf');
  if (!bookshelfContainer) return;

  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  // Clear container first
  bookshelfContainer.innerHTML = '';

  // Show loading indicator
  bookshelfContainer.innerHTML = `
    <div class="flex items-center justify-center h-full">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    // 1. Fetch library details to know the mediaType
    const lib = await request('GET', `/api/libraries/${libraryId}`);
    
    // 2. Fetch personalized shelves
    const shelves = await request('GET', `/api/libraries/${libraryId}/personalized`);
    
    const filterSelect = document.getElementById('filter-select');
    const sortSelect = document.getElementById('sort-select');
    const sortOrderToggle = document.getElementById('sort-order-toggle-btn');

    const searchInput = document.getElementById('global-search-input');
    const searchTerm = searchInput ? searchInput.value.trim() : '';

    let activeFilter = filterBy || localStorage.getItem('library-filterBy') || '';
    let activeSort = localStorage.getItem('library-sortBy') || 'media.metadata.title';
    let activeSortDesc = localStorage.getItem('library-sortDesc') === 'true';

    // 3. Fetch all items (up to 100 if filtered/searched, 40 if not)
    const limit = (activeFilter || searchTerm) ? 100 : 40;
    let url = `/api/libraries/${libraryId}/items?limit=${limit}&minified=1`;
    if (activeFilter) {
      url += `&filter=${encodeURIComponent(activeFilter)}`;
    }
    if (searchTerm) {
      url += `&search=${encodeURIComponent(searchTerm)}`;
    }
    if (activeSort) {
      url += `&sort=${encodeURIComponent(activeSort)}`;
    }
    if (activeSortDesc) {
      url += `&desc=1`;
    }
    const allItemsPayload = await request('GET', url);
    
    bookshelfContainer.innerHTML = '';

    const totalItems = allItemsPayload.total || 0;
    
    // Update count in toolbar
    const bookCountEl = document.getElementById('book-count');
    if (bookCountEl) {
      const unit = lib.mediaType === 'podcast' ? 'Podcasts' : 'Books';
      bookCountEl.textContent = `${totalItems} ${unit}`;
    }

    if (opmlBtn) {
      if (lib.mediaType === 'podcast') {
        opmlBtn.classList.remove('hidden');
        opmlBtn.onclick = () => {
          import('./opml.js').then(module => {
            module.openOPMLModal(libraryId);
          });
        };
      } else {
        opmlBtn.classList.add('hidden');
      }
    }

    const noItems = shelves.length === 0 && (!allItemsPayload.results || allItemsPayload.results.length === 0);
    const noFilteredItems = (filterBy || searchTerm) && (!allItemsPayload.results || allItemsPayload.results.length === 0);

    if (noItems || noFilteredItems) {
      bookshelfContainer.innerHTML = `
        <div class="flex flex-col items-center justify-center h-48 text-black-100">
          <span class="material-symbols text-4xl mb-2">library_books</span>
          <p class="text-sm font-medium">${(filterBy || searchTerm) ? 'No matching items found' : 'No items found in this library'}</p>
        </div>
      `;
      return;
    }

    const activeStyle = localStorage.getItem('library-style') || 'shelf';

    if (activeStyle === 'grid') {
      const gridContainer = document.createElement('div');
      gridContainer.className = 'library-grid w-full';
      
      allItemsPayload.results.forEach(item => {
        const card = createCard(item, false, libraryId);
        card.classList.remove('w-28e', 'h-40e', 'mr-8e');
        card.classList.add('w-full');
        card.style.width = 'var(--bookshelf-card-width)';
        card.style.height = 'var(--bookshelf-card-height)';
        gridContainer.appendChild(card);
      });
      bookshelfContainer.appendChild(gridContainer);
    } else if (activeStyle === 'list') {
      const tableWrapper = document.createElement('div');
      tableWrapper.className = 'w-full bg-primary/30 border border-black-400/40 rounded-lg overflow-hidden shadow-lg p-2 text-white';
      
      const table = document.createElement('table');
      table.className = 'w-full text-left text-xs';
      
      const visibleCols = getVisibleColumns();
      
      let headerHtml = `<tr class="border-b border-black-600/50 text-black-100 font-semibold uppercase tracking-wider">`;
      
      const colDetails = {
        cover: `<th class="p-3 w-16">Cover</th>`,
        title: `<th class="p-3">Title</th>`,
        author: `<th class="p-3">Author</th>`,
        narrator: `<th class="p-3">Narrator</th>`,
        series: `<th class="p-3">Series</th>`,
        duration: `<th class="p-3 font-mono">Duration</th>`,
        dateAdded: `<th class="p-3">Date Added</th>`,
        year: `<th class="p-3">Year</th>`,
        progress: `<th class="p-3">Progress</th>`,
        action: `<th class="p-3 w-20 text-center relative">
          <div class="inline-flex items-center space-x-1.5 justify-center w-full">
            <span>Action</span>
            <button id="customize-columns-btn" class="hover:text-white text-black-200 transition-colors focus:outline-none flex items-center cursor-pointer" title="Customize Columns">
              <span class="material-symbols text-[14px]">settings</span>
            </button>
          </div>
          <div id="columns-dropdown-menu" class="hidden absolute right-0 mt-2 w-48 bg-primary border border-black-400/60 rounded-md shadow-2xl z-[90] p-3 text-white text-left font-normal normal-case">
            <div class="text-[10px] font-bold text-black-100 mb-2 uppercase tracking-wider">Visible Columns</div>
            <div class="space-y-1.5" id="columns-checkboxes-container"></div>
          </div>
        </th>`
      };

      visibleCols.forEach(col => {
        headerHtml += colDetails[col] || '';
      });
      
      headerHtml += `</tr>`;
      
      table.innerHTML = `
        <thead>
          ${headerHtml}
        </thead>
        <tbody></tbody>
      `;
      
      const tbody = table.querySelector('tbody');
      allItemsPayload.results.forEach(item => {
        const tr = createListRow(item, libraryId, visibleCols);
        tbody.appendChild(tr);
      });
      
      tableWrapper.appendChild(table);
      bookshelfContainer.appendChild(tableWrapper);

      // Wire Customize Columns Dropdown
      const customizeBtn = table.querySelector('#customize-columns-btn');
      const dropdownMenu = table.querySelector('#columns-dropdown-menu');
      if (customizeBtn && dropdownMenu) {
        let isOpen = false;
        
        customizeBtn.onclick = (e) => {
          e.stopPropagation();
          if (isOpen) {
            dropdownMenu.classList.add('hidden');
            isOpen = false;
          } else {
            // Render checkboxes
            const container = dropdownMenu.querySelector('#columns-checkboxes-container');
            container.innerHTML = ALL_COLUMNS.map(col => {
              const isChecked = visibleCols.includes(col.key);
              const isMandatory = col.key === 'title' || col.key === 'action';
              return `
                <label class="flex items-center space-x-2 text-xs cursor-pointer select-none py-0.5">
                  <input type="checkbox" data-col="${col.key}" ${isChecked ? 'checked' : ''} ${isMandatory ? 'disabled' : ''} class="col-checkbox rounded border-black-400 bg-black-600 text-accent focus:ring-accent w-3.5 h-3.5">
                  <span class="${isMandatory ? 'text-black-300 font-medium' : 'text-black-100'}">${col.label}</span>
                </label>
              `;
            }).join('');
            
            // Wire checkbox changes
            container.querySelectorAll('.col-checkbox').forEach(cb => {
              cb.onchange = () => {
                const checkedCols = [];
                container.querySelectorAll('.col-checkbox').forEach(input => {
                  if (input.checked) {
                    checkedCols.push(input.getAttribute('data-col'));
                  }
                });
                saveVisibleColumns(checkedCols);
                loadDashboard(libraryId);
              };
            });

            dropdownMenu.classList.remove('hidden');
            dropdownMenu.offsetHeight; // reflow
            dropdownMenu.classList.remove('scale-95', 'opacity-0');
            dropdownMenu.classList.add('scale-100', 'opacity-100');
            isOpen = true;
          }
        };

        // Close on outside click
        document.addEventListener('click', () => {
          dropdownMenu.classList.add('hidden');
          isOpen = false;
        });
        dropdownMenu.onclick = (e) => e.stopPropagation();
      }
    } else {
      // Render personalized shelves only if not filtering/searching
      if (!filterBy && !searchTerm) {
        shelves.forEach(shelf => {
          if (shelf.entities && shelf.entities.length > 0) {
            const section = createShelfSection(shelf.id, shelf.label, shelf.entities, libraryId);
            bookshelfContainer.appendChild(section);
          }
        });
      }

      // Render "All Books" / "All Podcasts" shelf
      if (allItemsPayload.results && allItemsPayload.results.length > 0) {
        const allLabel = filterLabel || (lib.mediaType === 'podcast' ? 'All Podcasts' : 'All Books');
        const section = createShelfGridSection('all-books', allLabel, allItemsPayload.results, libraryId);
        bookshelfContainer.appendChild(section);
      }
    }

    initBatchEditHandlers(libraryId);

  } catch (err) {
    console.error('Failed to load dashboard:', err);
    bookshelfContainer.innerHTML = `
      <div class="flex flex-col items-center justify-center h-48 text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load library items</p>
      </div>
    `;
  }
}

function createShelfSection(shelfId, label, entities, libraryId) {
  const shelfWrapper = document.createElement('div');
  shelfWrapper.className = 'relative w-full';
  
  const rowDiv = document.createElement('div');
  rowDiv.className = 'w-full h-56 relative overflow-x-auto no-scroll overflow-y-hidden z-10 bg-repeat-x bookshelfRow';
  
  const itemsContainer = document.createElement('div');
  itemsContainer.id = `${shelfId}-shelf`;
  itemsContainer.className = 'w-max h-full pt-4e flex items-center pl-8e pr-8e';
  
  entities.forEach(item => {
    const card = createCard(item, shelfId.startsWith('continue'), libraryId);
    itemsContainer.appendChild(card);
  });
  
  rowDiv.appendChild(itemsContainer);
  shelfWrapper.appendChild(rowDiv);
  
  const plaqueDiv = document.createElement('div');
  plaqueDiv.className = 'relative h-12';
  plaqueDiv.innerHTML = `
    <div class="relative text-center categoryPlacard z-30 top-0 w-44e rounded-md mx-auto">
      <div class="shinyBlack flex items-center justify-center border rounded px-2 py-1">
        <h3 class="text-[0.85em] font-semibold tracking-wider font-mono">${label.toUpperCase()}</h3>
      </div>
    </div>
    <div class="bookshelfDividerCategorized h-6e w-full absolute top-0 left-0 right-0 z-20"></div>
  `;
  shelfWrapper.appendChild(plaqueDiv);
  
  return shelfWrapper;
}

function createShelfGridSection(shelfId, label, entities, libraryId) {
  const shelfWrapper = document.createElement('div');
  shelfWrapper.className = 'relative w-full flex flex-col mt-4';
  
  const plaqueDiv = document.createElement('div');
  plaqueDiv.className = 'relative h-12 mb-2';
  plaqueDiv.innerHTML = `
    <div class="relative text-center categoryPlacard z-30 top-0 w-44e rounded-md mx-auto">
      <div class="shinyBlack flex items-center justify-center border rounded px-2 py-1">
        <h3 class="text-[0.85em] font-semibold tracking-wider font-mono">${label.toUpperCase()}</h3>
      </div>
    </div>
  `;
  shelfWrapper.appendChild(plaqueDiv);

  const gridDiv = document.createElement('div');
  gridDiv.className = 'library-shelf-grid w-full z-10';
  
  entities.forEach(item => {
    const card = createCard(item, false, libraryId);
    card.classList.remove('w-28e', 'h-40e', 'mr-8e');
    gridDiv.appendChild(card);
  });
  
  shelfWrapper.appendChild(gridDiv);
  return shelfWrapper;
}

export function createCard(item, isContinue, libraryId) {
  const card = document.createElement('div');
  card.className = 'w-28e h-40e mr-8e relative cursor-pointer select-none box-shadow-book rounded-sm overflow-hidden flex-shrink-0 transition-transform hover:scale-105 group';
  
  let title = '';
  let author = '';
  if (item.mediaType === 'book') {
    const metadata = item.media?.metadata || {};
    title = metadata.title || item.title || 'Untitled';
    author = metadata.authorName || 'Unknown';
  } else if (item.mediaType === 'podcast') {
    const metadata = item.media?.metadata || {};
    title = metadata.title || item.title || 'Untitled';
    author = metadata.author || 'Unknown';
  } else {
    title = item.title || 'Untitled';
    author = 'Unknown';
  }

  const token = localStorage.getItem('token');
  const ts = item.updatedAt || item.addedAt || Date.now();
  const coverUrl = resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);
  const narrator = item.media?.metadata?.narratorName || '';

  card.innerHTML = `
    <img class="w-full h-full object-cover" src="${coverUrl}" alt="${escapeHtml(title)}" onerror="this.onerror=null; this.src='assets/images/logo.png'">
    
    <!-- Hover overlay details -->
    <div class="absolute inset-0 bg-black/85 opacity-0 group-hover:opacity-100 transition-opacity duration-200 flex flex-col justify-between p-3 select-none text-left z-30">
      <div class="overflow-y-auto no-scroll">
        <h4 class="font-semibold text-sm text-white leading-tight mb-1">${escapeHtml(title)}</h4>
        <p class="text-xs text-black-100">${escapeHtml(author)}</p>
        ${narrator ? `<p class="text-[10px] text-accent mt-2 italic truncate" title="${escapeHtml(narrator)}">Narrated by: ${escapeHtml(narrator)}</p>` : ''}
      </div>
    </div>
  `;

  if (isContinue) {
    const progBarContainer = document.createElement('div');
    progBarContainer.className = 'absolute bottom-0 left-0 right-0 h-1.5 bg-black/40 box-shadow-progressbar rounded-b-sm overflow-hidden z-20 hidden';
    const progBarFill = document.createElement('div');
    progBarFill.className = 'h-full bg-accent';
    progBarFill.style.width = '0%';
    progBarContainer.appendChild(progBarFill);
    card.appendChild(progBarContainer);

    // Fetch progress asynchronously
    request('GET', `/api/me/progress/${item.id}`)
      .then(progressObj => {
        if (progressObj && progressObj.progress !== undefined) {
          const percent = Math.min(Math.max(progressObj.progress * 100, 0), 100);
          progBarFill.style.width = `${percent}%`;
          progBarContainer.classList.remove('hidden');
          
          const overlayDiv = card.querySelector('.group-hover\\:opacity-100');
          if (overlayDiv) {
            const container = overlayDiv.querySelector('div');
            if (container) {
              const progressText = document.createElement('p');
              progressText.className = 'text-xs text-accent mt-2';
              progressText.textContent = `${Math.round(percent)}% completed`;
              container.appendChild(progressText);
            }
          }
        }
      })
      .catch(err => {
        console.warn(`Failed to fetch progress for item ${item.id}:`, err);
      });
  }

  if (batchEditMode && selectedItems.has(item.id)) {
    card.classList.add('ring-4', 'ring-accent', 'scale-105');
  }

  // Click handler to view item details
  card.addEventListener('click', (e) => {
    if (batchEditMode) {
      e.preventDefault();
      e.stopPropagation();
      if (selectedItems.has(item.id)) {
        selectedItems.delete(item.id);
        card.classList.remove('ring-4', 'ring-accent', 'scale-105');
      } else {
        selectedItems.add(item.id);
        card.classList.add('ring-4', 'ring-accent', 'scale-105');
      }
      updateBatchActionBar();
      return;
    }
    if (window.navigateTo) {
      window.navigateTo('/item/' + item.id);
    } else {
      loadItemDetails(item.id, libraryId, () => loadDashboard(libraryId));
    }
  });

  return card;
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

function updateBatchActionBar() {
  const bar = document.getElementById('batch-action-bar');
  const countEl = document.getElementById('batch-selected-count');
  const editBtn = document.getElementById('batch-edit-metadata-btn');
  if (!bar || !countEl) return;

  if (batchEditMode) {
    bar.classList.remove('hidden');
    countEl.textContent = `${selectedItems.size} items selected`;
    if (selectedItems.size > 0) {
      editBtn.disabled = false;
      editBtn.classList.remove('opacity-50', 'cursor-not-allowed');
    } else {
      editBtn.disabled = true;
      editBtn.classList.add('opacity-50', 'cursor-not-allowed');
    }
  } else {
    bar.classList.add('hidden');
  }
}

export function initBatchEditHandlers(libraryId) {
  const toggleBtn = document.getElementById('batch-edit-toggle-btn');
  if (!toggleBtn) return;

  toggleBtn.onclick = () => {
    batchEditMode = !batchEditMode;
    selectedItems.clear();
    updateBatchActionBar();

    const spanIcon = toggleBtn.querySelector('span');
    const textNode = toggleBtn.querySelector('#batch-edit-toggle-text') || toggleBtn.childNodes[1];

    if (batchEditMode) {
      if (spanIcon) spanIcon.textContent = 'close';
      if (textNode) textNode.textContent = 'Cancel';
      document.querySelectorAll('.bookshelfRow .group').forEach(el => {
        el.classList.add('hover:ring-2', 'hover:ring-accent/50');
      });
    } else {
      if (spanIcon) spanIcon.textContent = 'checklist';
      if (textNode) textNode.textContent = 'Batch Edit';
      document.querySelectorAll('.bookshelfRow .group').forEach(el => {
        el.classList.remove('hover:ring-2', 'hover:ring-accent/50', 'ring-4', 'ring-accent', 'scale-105');
      });
    }
  };

  const cancelBtn = document.getElementById('batch-cancel-btn');
  if (cancelBtn) {
    cancelBtn.onclick = () => {
      batchEditMode = false;
      selectedItems.clear();
      updateBatchActionBar();
      const spanIcon = toggleBtn.querySelector('span');
      const textNode = toggleBtn.querySelector('#batch-edit-toggle-text') || toggleBtn.childNodes[1];
      if (spanIcon) spanIcon.textContent = 'checklist';
      if (textNode) textNode.textContent = 'Batch Edit';
      document.querySelectorAll('.bookshelfRow .group').forEach(el => {
        el.classList.remove('hover:ring-2', 'hover:ring-accent/50', 'ring-4', 'ring-accent', 'scale-105');
      });
    };
  }

  const editBtn = document.getElementById('batch-edit-metadata-btn');
  if (editBtn) {
    editBtn.onclick = () => {
      if (selectedItems.size === 0) return;
      triggerBatchEditModal(Array.from(selectedItems), libraryId);
    };
  }
}

function triggerBatchEditModal(itemIds, libraryId) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 select-none';
  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-2xl p-6 rounded-md shadow-2xl space-y-4 flex flex-col max-h-[90vh]">
      <!-- Header -->
      <div class="flex justify-between items-center border-b border-black-400 pb-2 flex-shrink-0">
        <h3 class="text-lg font-bold text-white flex items-center space-x-2">
          <span class="material-symbols text-accent">edit_note</span>
          <span>Batch Edit Metadata (${itemIds.length} items selected)</span>
        </h3>
        <button id="close-batch-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-xl">close</span>
        </button>
      </div>

      <p class="text-xs text-black-100">
        Check the checkbox next to any field you wish to apply/overwrite across all selected items.
      </p>

      <!-- Form -->
      <form id="batch-edit-form" class="space-y-4 overflow-y-auto no-scroll pr-1 flex-grow">
        <!-- Tags -->
        <div class="flex items-start space-x-3">
          <input type="checkbox" id="batch-chk-tags" class="mt-1.5 w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
          <div class="flex-grow">
            <label for="batch-tags" class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1.5">Tags (comma-separated)</label>
            <input type="text" id="batch-tags" placeholder="e.g. History, Biography" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>

        <!-- Genres -->
        <div class="flex items-start space-x-3">
          <input type="checkbox" id="batch-chk-genres" class="mt-1.5 w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
          <div class="flex-grow">
            <label for="batch-genres" class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1.5">Genres (comma-separated)</label>
            <input type="text" id="batch-genres" placeholder="e.g. Science Fiction, Mystery" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>

        <!-- Authors -->
        <div class="flex items-start space-x-3">
          <input type="checkbox" id="batch-chk-authors" class="mt-1.5 w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
          <div class="flex-grow">
            <label for="batch-authors" class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1.5">Authors (comma-separated)</label>
            <input type="text" id="batch-authors" placeholder="e.g. Stephen King, Brandon Sanderson" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>

        <!-- Narrators -->
        <div class="flex items-start space-x-3">
          <input type="checkbox" id="batch-chk-narrators" class="mt-1.5 w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
          <div class="flex-grow">
            <label for="batch-narrators" class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1.5">Narrators (comma-separated)</label>
            <input type="text" id="batch-narrators" placeholder="e.g. Frank Muller, Roy Avers" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>

        <!-- Series / Sequence -->
        <div class="flex items-start space-x-3">
          <input type="checkbox" id="batch-chk-series" class="mt-1.5 w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
          <div class="flex-grow grid grid-cols-2 gap-4">
            <div>
              <label for="batch-series" class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1.5">Series</label>
              <input type="text" id="batch-series" placeholder="e.g. The Dark Tower" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>
            <div>
              <label for="batch-sequence" class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1.5">Sequence</label>
              <input type="text" id="batch-sequence" placeholder="e.g. 1" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>
          </div>
        </div>

        <!-- Publisher / Published Year -->
        <div class="flex items-start space-x-3">
          <input type="checkbox" id="batch-chk-publisher" class="mt-1.5 w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
          <div class="flex-grow grid grid-cols-2 gap-4">
            <div>
              <label for="batch-publisher" class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1.5">Publisher</label>
              <input type="text" id="batch-publisher" placeholder="e.g. Penguin Books" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>
            <div>
              <label for="batch-pubyear" class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1.5">Published Year</label>
              <input type="text" id="batch-pubyear" placeholder="e.g. 2023" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>
          </div>
        </div>

        <!-- Explicit / Abridged Flags -->
        <div class="flex items-start space-x-3">
          <input type="checkbox" id="batch-chk-flags" class="mt-1.5 w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
          <div class="flex space-x-6 pt-1">
            <div class="flex items-center space-x-2">
              <input type="checkbox" id="batch-explicit" class="w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
              <label for="batch-explicit" class="text-xs font-semibold text-black-100 uppercase tracking-wider">Explicit</label>
            </div>
            <div class="flex items-center space-x-2">
              <input type="checkbox" id="batch-abridged" class="w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
              <label for="batch-abridged" class="text-xs font-semibold text-black-100 uppercase tracking-wider">Abridged</label>
            </div>
          </div>
        </div>
      </form>

      <!-- Footer -->
      <div class="flex justify-end space-x-2 pt-2 border-t border-black-400 flex-shrink-0">
        <button id="cancel-batch-btn" class="bg-black-400 hover:bg-black-300 text-white font-semibold px-4 py-2 rounded text-xs transition-colors">Cancel</button>
        <button id="save-batch-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity shadow">Apply Changes</button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  document.getElementById('close-batch-modal').onclick = closeModal;
  document.getElementById('cancel-batch-btn').onclick = closeModal;

  document.getElementById('save-batch-btn').onclick = async (e) => {
    e.preventDefault();

    const chkTags = document.getElementById('batch-chk-tags').checked;
    const chkGenres = document.getElementById('batch-chk-genres').checked;
    const chkAuthors = document.getElementById('batch-chk-authors').checked;
    const chkNarrators = document.getElementById('batch-chk-narrators').checked;
    const chkSeries = document.getElementById('batch-chk-series').checked;
    const chkPublisher = document.getElementById('batch-chk-publisher').checked;
    const chkFlags = document.getElementById('batch-chk-flags').checked;

    if (!chkTags && !chkGenres && !chkAuthors && !chkNarrators && !chkSeries && !chkPublisher && !chkFlags) {
      alert('Please check at least one field checkbox to update');
      return;
    }

    const mediaPayload = {};

    if (chkTags) {
      mediaPayload.tags = splitCommaList(document.getElementById('batch-tags').value);
    }
    if (chkGenres) {
      mediaPayload.genres = splitCommaList(document.getElementById('batch-genres').value);
    }
    if (chkAuthors) {
      mediaPayload.authors = splitCommaList(document.getElementById('batch-authors').value);
    }
    if (chkNarrators) {
      mediaPayload.narrators = splitCommaList(document.getElementById('batch-narrators').value);
    }
    if (chkSeries) {
      mediaPayload.seriesName = document.getElementById('batch-series').value.trim();
      mediaPayload.seriesSequence = document.getElementById('batch-sequence').value.trim();
    }
    if (chkPublisher) {
      mediaPayload.publisher = document.getElementById('batch-publisher').value.trim();
      mediaPayload.publishedYear = document.getElementById('batch-pubyear').value.trim();
    }
    if (chkFlags) {
      mediaPayload.explicit = document.getElementById('batch-explicit').checked;
      mediaPayload.abridged = document.getElementById('batch-abridged').checked;
    }

    const payload = itemIds.map(id => ({
      id,
      mediaPayload
    }));

    try {
      await request('POST', '/api/items/batch/update', payload);
      closeModal();
      showToast(`Successfully updated ${itemIds.length} items`, 'success');
      
      const batchCancelBtn = document.getElementById('batch-cancel-btn');
      if (batchCancelBtn) {
        batchCancelBtn.click();
      }
      loadDashboard(libraryId);
    } catch (err) {
      console.error(err);
      alert('Failed to update: ' + err.message);
    }
  };
}

function splitCommaList(str) {
  if (!str) return [];
  return str.split(',').map(s => s.trim()).filter(s => s.length > 0);
}

function formatDuration(seconds) {
  if (!seconds) return '00:00';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (h > 0) {
    return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
  }
  return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
}

function createListRow(item, libraryId, visibleCols = ['cover', 'title', 'author', 'series', 'duration', 'action']) {
  const tr = document.createElement('tr');
  tr.className = 'border-b border-black-600/30 hover:bg-black-500/40 cursor-pointer transition-colors';
  
  let title = '';
  let author = '';
  let narrator = '';
  let series = '';
  let duration = '';
  let year = '';
  
  if (item.mediaType === 'book') {
    const metadata = item.media?.metadata || {};
    title = metadata.title || item.title || 'Untitled';
    author = metadata.authorName || 'Unknown';
    narrator = (metadata.narrators || []).join(', ') || 'Unknown';
    if (metadata.seriesName) {
      series = metadata.seriesName + (metadata.sequence ? ` #${metadata.sequence}` : '');
    }
    const durSec = item.media?.duration || 0;
    duration = durSec ? formatDuration(durSec) : 'N/A';
    year = metadata.publishedYear || 'N/A';
  } else if (item.mediaType === 'podcast') {
    const metadata = item.media?.metadata || {};
    title = metadata.title || item.title || 'Untitled';
    author = metadata.author || 'Unknown';
    const durSec = item.media?.duration || 0;
    duration = durSec ? formatDuration(durSec) : 'N/A';
    year = metadata.publishedYear || 'N/A';
  }

  const token = localStorage.getItem('token');
  const ts = item.updatedAt || item.addedAt || Date.now();
  const coverUrl = resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);

  let rowHtml = '';

  visibleCols.forEach(col => {
    switch (col) {
      case 'cover':
        rowHtml += `
          <td class="p-3 w-16">
            <img src="${coverUrl}" class="w-10 h-14 object-cover rounded shadow border border-black-400/20" onerror="this.onerror=null; this.src='assets/images/logo.png'">
          </td>
        `;
        break;
      case 'title':
        rowHtml += `<td class="p-3 font-semibold text-white">${escapeHtml(title)}</td>`;
        break;
      case 'author':
        rowHtml += `<td class="p-3 text-black-50">${escapeHtml(author)}</td>`;
        break;
      case 'narrator':
        rowHtml += `<td class="p-3 text-black-100">${escapeHtml(narrator)}</td>`;
        break;
      case 'series':
        rowHtml += `<td class="p-3 text-accent/80 font-mono">${escapeHtml(series)}</td>`;
        break;
      case 'duration':
        rowHtml += `<td class="p-3 text-black-100 font-mono">${duration}</td>`;
        break;
      case 'dateAdded':
        const addedDate = item.addedAt ? new Date(item.addedAt).toLocaleDateString() : 'N/A';
        rowHtml += `<td class="p-3 text-black-100 font-mono">${addedDate}</td>`;
        break;
      case 'year':
        rowHtml += `<td class="p-3 text-black-100 font-mono">${year}</td>`;
        break;
      case 'progress':
        const progressId = `progress-${item.id}-${Math.random().toString(36).substr(2, 9)}`;
        rowHtml += `
          <td class="p-3 text-black-100 font-mono" id="${progressId}">
            <span class="text-black-300">...</span>
          </td>
        `;
        setTimeout(() => {
          const progressTd = document.getElementById(progressId);
          if (!progressTd) return;
          request('GET', `/api/me/progress/${item.id}`)
            .then(progressObj => {
              if (progressObj && progressObj.progress !== undefined) {
                if (progressObj.isFinished) {
                  progressTd.innerHTML = '<span class="text-green-500 font-semibold">Completed</span>';
                } else if (progressObj.progress === 0) {
                  progressTd.innerHTML = '<span class="text-black-200">Not Started</span>';
                } else {
                  const percent = Math.min(Math.max(progressObj.progress * 100, 0), 100);
                  progressTd.innerHTML = `<span class="text-accent">${Math.round(percent)}%</span>`;
                }
              } else {
                progressTd.innerHTML = '<span class="text-black-200">Not Started</span>';
              }
            })
            .catch(() => {
              progressTd.innerHTML = '<span class="text-black-200">Not Started</span>';
            });
        }, 0);
        break;
      case 'action':
        rowHtml += `
          <td class="p-3 text-center w-20">
            <button class="play-btn bg-accent text-primary w-8 h-8 rounded-full flex items-center justify-center hover:scale-105 transition-transform mx-auto" title="Play">
              <span class="material-symbols text-sm">play_arrow</span>
            </button>
          </td>
        `;
        break;
    }
  });

  tr.innerHTML = rowHtml;

  tr.onclick = (e) => {
    if (e.target.closest('.play-btn')) {
      e.stopPropagation();
      playItem(item.id);
      return;
    }
    window.history.pushState(null, '', resolvePath(`/item/${item.id}`));
    window.dispatchEvent(new CustomEvent('popstate'));
  };

  return tr;
}
