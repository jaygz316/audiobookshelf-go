// frontend/js/authors.js
// Provides Authors and Series listing views for the sidebar navigation.

import { request, resolvePath } from './api.js';

/**
 * Load and render the Authors listing view for the given library.
 * Fetches GET /api/libraries/{libraryId}/authors and renders a grid.
 */
export async function loadAuthors(libraryId) {
  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Authors';

  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = '';

  container.innerHTML = `
    <div class="flex items-center justify-center h-32">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    const payload = await request('GET', `/api/libraries/${libraryId}/authors`);
    const authors = payload.authors || [];

    if (bookCount) bookCount.textContent = `${authors.length} Authors`;

    if (authors.length === 0) {
      container.innerHTML = `
        <div class="flex flex-col items-center justify-center h-48 text-black-100">
          <span class="material-symbols text-4xl mb-2">person</span>
          <p class="text-sm font-medium">No authors found in this library</p>
        </div>
      `;
      return;
    }

    container.innerHTML = '';

    const grid = document.createElement('div');
    grid.className = 'grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 p-6';

    authors.forEach(author => {
      const card = createAuthorCard(author);
      grid.appendChild(card);
    });

    container.appendChild(grid);
  } catch (err) {
    console.error('Failed to load authors:', err);
    container.innerHTML = `
      <div class="flex flex-col items-center justify-center h-48 text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load authors: ${escapeHtml(err.message)}</p>
      </div>
    `;
  }
}

/**
 * Load and render the Series listing view for the given library.
 * Fetches GET /api/libraries/{libraryId}/series and renders a grid.
 */
export async function loadSeries(libraryId) {
  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Series';

  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = '';

  container.innerHTML = `
    <div class="flex items-center justify-center h-32">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    const payload = await request('GET', `/api/libraries/${libraryId}/series`);
    const seriesList = payload.results || payload.series || [];

    if (bookCount) bookCount.textContent = `${seriesList.length} Series`;

    if (seriesList.length === 0) {
      container.innerHTML = `
        <div class="flex flex-col items-center justify-center h-48 text-black-100">
          <span class="material-symbols text-4xl mb-2">layers</span>
          <p class="text-sm font-medium">No series found in this library</p>
        </div>
      `;
      return;
    }

    container.innerHTML = '';

    const grid = document.createElement('div');
    grid.className = 'grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 p-6';

    seriesList.forEach(series => {
      const card = createSeriesCard(series);
      grid.appendChild(card);
    });

    container.appendChild(grid);
  } catch (err) {
    console.error('Failed to load series:', err);
    container.innerHTML = `
      <div class="flex flex-col items-center justify-center h-48 text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load series: ${escapeHtml(err.message)}</p>
      </div>
    `;
  }
}

function createAuthorCard(author) {
  const card = document.createElement('div');
  card.className = 'flex flex-col items-center p-3 bg-primary border border-black-400 rounded-md hover:bg-black-500 cursor-pointer transition-colors group';

  const token = localStorage.getItem('token');
  const imageUrl = resolvePath(`/api/authors/${author.id}/image?token=${token}`);

  const numBooks = author.numBooks !== undefined ? author.numBooks : (author.bookCount || 0);

  card.innerHTML = `
    <div class="w-20 h-20 rounded-full overflow-hidden bg-black-400 mb-2 flex items-center justify-center flex-shrink-0">
      <img src="${imageUrl}" alt="${escapeHtml(author.name)}" class="w-full h-full object-cover">
    </div>
    <p class="text-sm font-semibold text-white text-center leading-tight truncate w-full text-center">${escapeHtml(author.name)}</p>
    <p class="text-xs text-black-100 mt-0.5">${numBooks} book${numBooks !== 1 ? 's' : ''}</p>
  `;

  const img = card.querySelector('img');
  if (img) {
    img.addEventListener('error', function() {
      const parent = this.parentElement;
      if (parent) {
        parent.innerHTML = '<span class="material-symbols text-4xl text-black-100">person</span>';
      }
    });
  }

  return card;
}

function createSeriesCard(series) {
  const card = document.createElement('div');
  card.className = 'flex flex-col items-center p-3 bg-primary border border-black-400 rounded-md hover:bg-black-500 cursor-pointer transition-colors group';

  const numBooks = series.numBooks !== undefined ? series.numBooks : (series.bookCount || 0);

  card.innerHTML = `
    <div class="w-20 h-20 rounded-md overflow-hidden bg-black-400 mb-2 flex items-center justify-center flex-shrink-0">
      <span class="material-symbols text-4xl text-black-100">layers</span>
    </div>
    <p class="text-sm font-semibold text-white text-center leading-tight truncate w-full text-center">${escapeHtml(series.name)}</p>
    <p class="text-xs text-black-100 mt-0.5">${numBooks} book${numBooks !== 1 ? 's' : ''}</p>
  `;

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
