// frontend/js/narrators.js
// Provides Narrators listing and filtering views for the sidebar navigation.

import { request } from './api.js';
import { getActiveLibraryId } from './library.js';

let currentSearch = '';
let currentSort = 'name'; // 'name' or 'numBooks'
let currentDesc = false;

/**
 * Load and render the Narrators listing view for the given library.
 * Fetches GET /api/libraries/{libraryId}/narrators and renders a grid with controls.
 */
export async function loadNarrators(libraryId) {
  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Narrators';

  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = '';

  // Show loading indicator
  container.innerHTML = `
    <div class="flex items-center justify-center h-32">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
    </div>
  `;

  renderNarratorsView(container, libraryId);
}

async function renderNarratorsView(container, libraryId) {
  try {
    // Build query params
    const queryParams = new URLSearchParams({
      sort: currentSort,
      desc: currentDesc ? 'true' : 'false',
      search: currentSearch
    });

    const payload = await request('GET', `/api/libraries/${libraryId}/narrators?${queryParams.toString()}`);
    const narrators = payload.narrators || payload.results || [];

    const bookCount = document.getElementById('book-count');
    if (bookCount) bookCount.textContent = `${narrators.length} Narrator${narrators.length !== 1 ? 's' : ''}`;

    // Render toolbar + grid container
    container.innerHTML = `
      <div class="p-6 space-y-6 text-left">
        <!-- Controls Toolbar -->
        <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-4 bg-black-600/40 p-4 rounded-lg border border-black-600/30">
          <!-- Search input -->
          <div class="relative flex-grow max-w-md">
            <span class="material-symbols absolute left-3 top-2.5 text-black-200 text-lg">search</span>
            <input type="text" id="narrators-search" placeholder="Search narrators..." value="${escapeHtml(currentSearch)}"
              class="w-full bg-black-500 text-white pl-10 pr-4 py-2 rounded-lg border border-black-300 focus:outline-none focus:border-accent text-sm transition-colors">
          </div>

          <!-- Sort and Order controls -->
          <div class="flex items-center gap-3">
            <label class="text-xs font-semibold text-black-100 uppercase tracking-wider">Sort by:</label>
            <select id="narrators-sort-select" class="bg-black-500 border border-black-300 text-white text-xs rounded px-3 py-1.5 focus:outline-none cursor-pointer">
              <option value="name" ${currentSort === 'name' ? 'selected' : ''}>Name</option>
              <option value="numBooks" ${currentSort === 'numBooks' ? 'selected' : ''}>Book Count</option>
            </select>
            <button id="narrators-direction-btn" class="p-1.5 bg-black-500 hover:bg-black-400 border border-black-300 rounded text-white flex items-center justify-center transition-colors" title="Toggle Sort Order">
              <span class="material-symbols text-lg">${currentDesc ? 'arrow_downward' : 'arrow_upward'}</span>
            </button>
          </div>
        </div>

        <!-- Narrators Grid -->
        <div id="narrators-grid-container"></div>
      </div>
    `;

    // Setup input/button event handlers
    const searchInput = document.getElementById('narrators-search');
    if (searchInput) {
      // Debounce search slightly
      let searchTimeout;
      searchInput.addEventListener('input', (e) => {
        currentSearch = e.target.value;
        clearTimeout(searchTimeout);
        searchTimeout = setTimeout(() => {
          loadNarrators(libraryId);
        }, 300);
      });
    }

    const sortSelect = document.getElementById('narrators-sort-select');
    if (sortSelect) {
      sortSelect.addEventListener('change', (e) => {
        currentSort = e.target.value;
        loadNarrators(libraryId);
      });
    }

    const directionBtn = document.getElementById('narrators-direction-btn');
    if (directionBtn) {
      directionBtn.addEventListener('click', () => {
        currentDesc = !currentDesc;
        loadNarrators(libraryId);
      });
    }

    // Render Grid
    const gridContainer = document.getElementById('narrators-grid-container');
    if (narrators.length === 0) {
      gridContainer.innerHTML = `
        <div class="flex flex-col items-center justify-center h-48 text-black-100">
          <span class="material-symbols text-4xl mb-2">record_voice_over</span>
          <p class="text-sm font-medium">No narrators found</p>
        </div>
      `;
      return;
    }

    const grid = document.createElement('div');
    grid.style.display = 'grid';
    grid.style.gridTemplateColumns = 'repeat(auto-fill, minmax(var(--bookshelf-card-width, 120px), 1fr))';
    grid.style.gap = '1.5rem';

    narrators.forEach(narrator => {
      const card = createNarratorCard(narrator);
      grid.appendChild(card);
    });

    gridContainer.appendChild(grid);
  } catch (err) {
    console.error('Failed to load narrators:', err);
    container.innerHTML = `
      <div class="flex flex-col items-center justify-center h-48 text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load narrators: ${escapeHtml(err.message)}</p>
      </div>
    `;
  }
}

function createNarratorCard(narrator) {
  const card = document.createElement('div');
  card.className = 'flex flex-col items-center p-3 bg-primary border border-black-400 rounded-md hover:bg-black-500 cursor-pointer transition-colors group';
  card.style.width = '100%';

  const numBooks = narrator.numBooks || 0;

  card.innerHTML = `
    <div class="w-2/3 aspect-square rounded-full bg-gradient-to-tr from-black-600 to-black-400 mb-3 flex items-center justify-center flex-shrink-0 group-hover:from-accent/20 group-hover:to-accent/5 transition-all">
      <span class="material-symbols text-3xl text-black-100 group-hover:text-accent transition-colors">record_voice_over</span>
    </div>
    <p class="text-xs font-semibold text-white text-center leading-tight truncate w-full group-hover:text-accent transition-colors" title="${escapeHtml(narrator.name)}">${escapeHtml(narrator.name)}</p>
    <p class="text-[10px] text-black-100 mt-1">${numBooks} book${numBooks !== 1 ? 's' : ''}</p>
  `;

  card.onclick = () => {
    window.dispatchEvent(new CustomEvent('navigate-to-dashboard', {
      detail: {
        filterBy: `narrators.${narrator.name}`,
        filterLabel: `Narrator: ${narrator.name}`
      }
    }));
  };

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
