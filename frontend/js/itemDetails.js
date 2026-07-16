import { triggerAddToPlaylistModal } from './modals/playlistModal.js';
import { triggerEditItemDetailsModal } from './modals/editDetailsModal.js';
import { triggerMatchBookModal, triggerMatchCoverModal, triggerMatchModal } from './modals/matchBookModal.js';
import { renderBookmarks, triggerEditBookmarkModal, triggerAddBookmarkOnDetailsModal, triggerExportBookmarksModal, triggerImportBookmarksModal } from './modals/bookmarksModal.js';
import { triggerEditChaptersModal } from './modals/chaptersModal.js';
import { triggerShareLinkModal } from './modals/shareModal.js';
import { triggerCoverEditorModal } from './modals/coverEditorModal.js';
import { request, resolvePath } from './api.js';
import { playItem, getCurrentPlayingItem, getCurrentPlaybackTime, addToQueue, seekTo } from './player.js';
import { openEbookReader } from './reader.js';
import { showToast } from './app.js';

let currentUser = null;
let activeItemId = null;
let activeLibraryId = null;
let activeBackCallback = null;
let currentBookmarkListener = null;

async function getCurrentUser() {
  if (currentUser) return currentUser;
  try {
    currentUser = await request('GET', '/api/me');
    return currentUser;
  } catch (err) {
    console.error('Failed to retrieve user details:', err);
    return null;
  }
}

/**
 * Loads the details for a specific library item and renders them.
 * @param {string} itemId - The ID of the item to load.
 * @param {string} libraryId - The active library ID.
 * @param {function} backCallback - Function to execute when clicking "Back".
 */
export async function loadItemDetails(itemId, libraryId, backCallback) {
  activeItemId = itemId;
  activeLibraryId = libraryId;
  activeBackCallback = backCallback;

  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  const container = document.getElementById('bookshelf');
  if (!container) return;

  container.innerHTML = `
    <div class="flex items-center justify-center h-full mt-20">
      <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    const item = await request('GET', `/api/items/${itemId}`);
    const user = await getCurrentUser();
    const isAdmin = user && (user.type === 'root' || user.type === 'admin');

    const token = localStorage.getItem('token');
    const ts = item.updatedAt || item.addedAt || Date.now();
    const coverUrl = resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);

    let title = 'Untitled';
    let subtitle = '';
    let authorName = 'Unknown';
    let narratorName = '';
    let seriesName = '';
    let seriesSequence = '';
    let description = '';
    let publisher = '';
    let publishedYear = '';
    let publishedDate = '';
    let isbn = '';
    let asin = '';
    let language = '';
    let explicit = false;
    let abridged = false;
    let tags = [];
    let genres = [];
    let mediaType = item.mediaType || 'book';

    if (item.media) {
      const metadata = item.media.metadata || {};
      title = metadata.title || item.title || title;
      subtitle = metadata.subtitle || '';
      authorName = metadata.authorName || metadata.author || authorName;
      narratorName = metadata.narratorName || (metadata.narrators ? metadata.narrators.join(', ') : '');
      description = metadata.description || '';
      publisher = metadata.publisher || '';
      publishedYear = metadata.publishedYear || '';
      publishedDate = metadata.publishedDate || '';
      isbn = metadata.isbn || '';
      asin = metadata.asin || '';
      language = metadata.language || '';
      explicit = !!metadata.explicit;
      abridged = !!metadata.abridged;
      tags = item.media.tags || [];
      genres = metadata.genres || [];

      if (metadata.series && metadata.series.length > 0) {
        seriesName = metadata.series[0].name || '';
        seriesSequence = metadata.series[0].sequence || '';
      } else if (metadata.seriesName) {
        seriesName = metadata.seriesName;
      }
    }

    const hasAudio = item.media && (
      item.media.duration > 0 || 
      (item.media.audioFiles && item.media.audioFiles.length > 0) ||
      (item.media.tracks && item.media.tracks.length > 0) ||
      (item.media.episodes && item.media.episodes.length > 0)
    );

    const hasEbook = item.media && (item.media.ebookFile || item.media.ebookFormat);

    // Update Toolbar details
    const viewTitle = document.getElementById('view-title');
    if (viewTitle) viewTitle.textContent = title;
    const bookCount = document.getElementById('book-count');
    if (bookCount) {
      bookCount.textContent = mediaType === 'podcast' ? 'Podcast' : 'Book';
    }

    container.innerHTML = `
      <div class="p-6 space-y-6 max-w-5xl mx-auto">
        <!-- Navigation Header -->
        <div class="flex items-center justify-between border-b border-black-600/50 pb-4">
          <button id="details-back-btn" class="flex items-center space-x-1.5 text-sm text-black-50 hover:text-white transition-colors">
            <span class="material-symbols">arrow_back</span>
            <span>Back</span>
          </button>
          ${isAdmin ? `
            <div class="flex items-center space-x-2">
              <button id="details-match-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold px-3 py-1.5 rounded text-xs flex items-center space-x-1 transition-colors">
                <span class="material-symbols text-sm">find_replace</span>
                <span>Match</span>
              </button>
              <button id="details-edit-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold px-3 py-1.5 rounded text-xs flex items-center space-x-1 transition-colors">
                <span class="material-symbols text-sm">edit</span>
                <span>Edit Details</span>
              </button>
              <button id="details-delete-item-btn" class="bg-black-500 hover:bg-red-950 border border-black-300 text-error hover:text-white font-semibold px-3 py-1.5 rounded text-xs flex items-center space-x-1 transition-colors">
                <span class="material-symbols text-sm">delete</span>
                <span>Delete</span>
              </button>
            </div>
          ` : ''}
        </div>

        <!-- Main Grid Layout -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-8">
          <!-- Left Column: Cover & Core Actions -->
          <div class="flex flex-col items-center space-y-4">
            <div id="details-cover-container" class="w-56 h-80 bg-black-500 rounded border border-black-400 overflow-hidden shadow-2xl flex-shrink-0 flex items-center justify-center relative group select-none cursor-pointer">
              <img src="${coverUrl}" alt="${escapeHtml(title)}" class="w-full h-full object-cover" onerror="this.onerror=null; this.src='assets/images/logo.png'">
              ${isAdmin ? `
                <div class="absolute inset-0 bg-black-950/70 opacity-0 group-hover:opacity-100 flex flex-col items-center justify-center transition-opacity duration-200">
                  <span class="material-symbols text-3xl text-white">edit</span>
                  <span class="text-xs text-white font-semibold mt-1">Change Cover</span>
                </div>
              ` : ''}
            </div>
            
            <!-- Core Play/Read Buttons -->
            <div class="w-full space-y-2 max-w-xs">
              ${hasAudio ? `
                <button id="details-play-action-btn" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-2.5 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-sm shadow hover:scale-[1.02] duration-200">
                  <span class="material-symbols text-lg font-bold">play_arrow</span>
                  <span>Play Audiobook</span>
                </button>
                <button id="details-queue-action-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                  <span class="material-symbols text-sm">playlist_add</span>
                  <span>Add to Queue</span>
                </button>
              ` : ''}
              
              ${hasEbook ? `
                <button id="details-read-action-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-bold py-2.5 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-sm shadow hover:scale-[1.02] duration-200">
                  <span class="material-symbols text-lg font-bold">menu_book</span>
                  <span>Read Book</span>
                </button>
                <button id="details-send-device-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                  <span class="material-symbols text-sm">send_to_mobile</span>
                  <span>Send to Device</span>
                </button>
              ` : ''}

              <!-- Playlist Button -->
              <button id="details-playlist-action-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                <span class="material-symbols text-sm">playlist_add</span>
                <span>Add to Playlist</span>
              </button>

              <!-- Download Button -->
              ${(user && user.permissions?.download) || isAdmin ? `
                <button id="details-download-action-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                  <span class="material-symbols text-sm">download</span>
                  <span>Download</span>
                </button>
              ` : ''}
              
              ${isAdmin ? `
                <button id="details-match-cover-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200">
                  <span class="material-symbols text-sm">image</span>
                  <span>${item.media?.coverPath ? 'Change Cover' : 'Get Cover Art'}</span>
                </button>
                <button id="details-embed-metadata-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                  <span class="material-symbols text-sm">settings_suggest</span>
                  <span>Embed Metadata</span>
                </button>
                ${item.media?.audioFiles && item.media.audioFiles.length > 1 ? `
                  <button id="details-merge-audio-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                    <span class="material-symbols text-sm">call_merge</span>
                    <span>Merge Audio Files</span>
                  </button>
                ` : ''}
                <button id="details-share-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                  <span class="material-symbols text-sm">share</span>
                  <span>Share Link</span>
                </button>
              ` : ''}
            </div>

            <!-- Progress Section -->
            <div id="details-progress-section" class="w-full max-w-xs border border-black-400/50 bg-primary/20 rounded-md p-3.5 space-y-3 text-xs text-left hidden">
              <div class="flex items-center justify-between border-b border-black-500 pb-2">
                <span class="font-bold text-white uppercase tracking-wider">Your Progress</span>
                <span id="progress-status-badge" class="px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-black-500 text-black-100">Not Started</span>
              </div>
              <div class="space-y-2">
                <!-- Progress Bar -->
                <div class="w-full bg-black-600 rounded-full h-2 overflow-hidden relative">
                  <div id="progress-bar-fill" class="h-full bg-accent" style="width: 0%;"></div>
                </div>
                
                <div class="flex justify-between items-center text-[10px] text-black-100 font-semibold">
                  <span id="progress-time-listened">00:00:00</span>
                  <span id="progress-percent">0%</span>
                  <span id="progress-time-remaining">00:00:00 left</span>
                </div>

                <div class="flex space-x-2 pt-2">
                  <button id="progress-toggle-finished-btn" class="flex-grow bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-1.5 px-3 rounded text-[10px] flex items-center justify-center space-x-1 transition-all">
                    <span class="material-symbols text-xs">check_circle</span>
                    <span id="progress-toggle-finished-text">Mark Finished</span>
                  </button>
                  <button id="progress-reset-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-error font-semibold py-1.5 px-2 rounded text-[10px] flex items-center justify-center transition-all" title="Reset Progress">
                    <span class="material-symbols text-xs">restart_alt</span>
                  </button>
                </div>
              </div>
            </div>

            <!-- RSS Feed Status & Management Section -->
            <div id="details-rss-section" class="w-full max-w-xs border border-black-400/50 bg-primary/20 rounded-md p-3.5 space-y-3 text-xs text-left">
              <div class="flex items-center justify-between border-b border-black-500 pb-2">
                <span class="font-bold text-white uppercase tracking-wider">RSS Feed</span>
                <span id="rss-status-badge" class="px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-black-500 text-black-100">Closed</span>
              </div>
              <div id="rss-controls" class="space-y-2">
                <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-accent mx-auto"></div>
              </div>
            </div>
          </div>

          <!-- Middle & Right Columns: Metadata & Info -->
          <div class="md:col-span-2 space-y-6 text-left">
            <div>
              <h2 class="text-3xl font-bold text-white tracking-wide">${escapeHtml(title)}</h2>
              ${subtitle ? `<p class="text-base text-black-50 mt-1">${escapeHtml(subtitle)}</p>` : ''}
            </div>

            <!-- Author & Series Line -->
            <div class="flex flex-wrap gap-x-6 gap-y-2 text-sm text-black-100">
              <div class="flex items-center space-x-1">
                <span class="material-symbols text-base text-accent">person</span>
                ${(item.media?.metadata?.authors && item.media.metadata.authors.length > 0) ?
                  item.media.metadata.authors.map((auth, idx) => `
                    <span class="author-link font-medium text-white hover:text-accent cursor-pointer transition-colors hover:underline" data-id="${auth.id}" data-name="${escapeHtml(auth.name)}">${escapeHtml(auth.name)}</span>${idx < item.media.metadata.authors.length - 1 ? '<span class="text-black-100">, </span>' : ''}
                  `).join('') : `<span class="font-medium text-white">${escapeHtml(authorName)}</span>`
                }
              </div>
              ${(item.media?.metadata?.series && item.media.metadata.series.length > 0) ?
                item.media.metadata.series.map(ser => `
                  <div class="flex items-center space-x-1">
                    <span class="material-symbols text-base text-accent">layers</span>
                    <span>Series: <span class="series-link font-medium text-white hover:text-accent cursor-pointer transition-colors hover:underline" data-id="${ser.id}" data-name="${escapeHtml(ser.name)}">${escapeHtml(ser.name)}</span> ${ser.sequence ? `(Book ${ser.sequence})` : ''}</span>
                  </div>
                `).join('') : (seriesName ? `
                  <div class="flex items-center space-x-1">
                    <span class="material-symbols text-base text-accent">layers</span>
                    <span>Series: <span class="font-medium text-white">${escapeHtml(seriesName)}</span> ${seriesSequence ? `(Book ${seriesSequence})` : ''}</span>
                  </div>
                ` : '')
              }
              ${(item.media?.metadata?.narrators && item.media.metadata.narrators.length > 0) ? `
                <div class="flex items-center space-x-1">
                  <span class="material-symbols text-base text-accent">record_voice_over</span>
                  <span>Narrator: ${item.media.metadata.narrators.map((narrator, idx) => `
                    <span class="narrator-link font-medium text-white hover:text-accent cursor-pointer transition-colors hover:underline" data-name="${escapeHtml(narrator)}">${escapeHtml(narrator)}</span>${idx < item.media.metadata.narrators.length - 1 ? '<span class="text-black-100">, </span>' : ''}
                  `).join('')}</span>
                </div>
              ` : (narratorName ? `
                <div class="flex items-center space-x-1">
                  <span class="material-symbols text-base text-accent">record_voice_over</span>
                  <span>Narrator: <span class="narrator-link font-medium text-white hover:text-accent cursor-pointer transition-colors hover:underline" data-name="${escapeHtml(narratorName)}">${escapeHtml(narratorName)}</span></span>
                </div>
              ` : '')}
            </div>

            <!-- Description -->
            ${description ? `
              <div class="bg-primary border border-black-300 rounded-md p-4 space-y-2">
                <h3 class="text-xs font-semibold uppercase tracking-wider text-black-100">Description</h3>
                <p class="text-sm text-black-50 leading-relaxed whitespace-pre-line overflow-y-auto max-h-48 no-scroll">${escapeHtml(description)}</p>
              </div>
            ` : ''}

            <!-- Metadata Grid -->
            <div class="grid grid-cols-2 md:grid-cols-3 gap-4 text-xs bg-primary/40 border border-black-400/50 rounded-md p-4">
              ${publisher ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Publisher</p>
                  <p class="text-white mt-0.5 text-sm">${escapeHtml(publisher)}</p>
                </div>
              ` : ''}
              ${publishedYear || publishedDate ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Published</p>
                  <p class="text-white mt-0.5 text-sm">${escapeHtml(publishedDate || publishedYear)}</p>
                </div>
              ` : ''}
              ${language ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Language</p>
                  <p class="text-white mt-0.5 text-sm">${escapeHtml(language)}</p>
                </div>
              ` : ''}
              ${isbn ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">ISBN</p>
                  <p class="text-white mt-0.5 text-sm">${escapeHtml(isbn)}</p>
                </div>
              ` : ''}
              ${asin ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">ASIN</p>
                  <p class="text-white mt-0.5 text-sm">${escapeHtml(asin)}</p>
                </div>
              ` : ''}
              ${item.size ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Size</p>
                  <p class="text-white mt-0.5 text-sm">${formatBytes(item.size)}</p>
                </div>
              ` : ''}
              ${item.media && item.media.duration ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Duration</p>
                  <p class="text-white mt-0.5 text-sm">${formatDuration(item.media.duration)}</p>
                </div>
              ` : ''}
              <div>
                <p class="text-black-100 uppercase font-semibold">Explicit</p>
                <p class="text-white mt-0.5 text-sm">${explicit ? 'Yes' : 'No'}</p>
              </div>
              ${mediaType === 'book' ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Abridged</p>
                  <p class="text-white mt-0.5 text-sm">${abridged ? 'Yes' : 'No'}</p>
                </div>
              ` : ''}
              ${isAdmin && item.path ? `
                <div class="col-span-2 md:col-span-3">
                  <p class="text-black-100 uppercase font-semibold">Path</p>
                  <p class="text-white mt-0.5 text-xs font-mono break-all select-all">${escapeHtml(item.path)}</p>
                </div>
              ` : ''}
            </div>

            <!-- Genres & Tags -->
            <div class="space-y-3">
              ${genres.length > 0 ? `
                <div class="space-y-1">
                  <h4 class="text-xs uppercase font-semibold text-black-100">Genres</h4>
                  <div class="flex flex-wrap gap-2">
                    ${genres.map(g => `<span class="genre-link bg-black-500 border border-black-300 text-black-50 px-2.5 py-0.5 rounded-full text-xs font-medium cursor-pointer hover:text-accent hover:border-accent/40 hover:bg-black-500/80 transition-all" data-name="${escapeHtml(g)}">${escapeHtml(g)}</span>`).join('')}
                  </div>
                </div>
              ` : ''}
              
              ${tags.length > 0 ? `
                <div class="space-y-1">
                  <h4 class="text-xs uppercase font-semibold text-black-100">Tags</h4>
                  <div class="flex flex-wrap gap-2">
                    ${tags.map(t => `<span class="tag-link bg-accent/10 border border-accent/20 text-accent px-2.5 py-0.5 rounded-full text-xs font-medium cursor-pointer hover:text-white hover:border-accent/60 hover:bg-accent/20 transition-all" data-name="${escapeHtml(t)}">${escapeHtml(t)}</span>`).join('')}
                  </div>
                </div>
              ` : ''}

            </div>

            <!-- Tracks / Episode Accordion -->
            ${mediaType === 'podcast' && item.media ? `
              <div class="space-y-3" id="podcast-episodes-section">
                <!-- Managed dynamically in JS -->
              </div>
            ` : ''}

            ${mediaType === 'podcast' ? `
              <div class="space-y-3 bg-black-500/10 p-3.5 rounded border border-black-400/20">
                <h3 class="font-bold text-xs text-white border-b border-black-400 pb-1 flex items-center gap-1.5 uppercase tracking-wider">
                  <span class="material-symbols text-sm text-accent">settings</span>
                  Podcast Settings
                </h3>
                
                <!-- Auto download toggle -->
                <div class="flex items-center justify-between">
                  <div class="flex flex-col pr-2">
                    <span class="text-[10px] font-bold text-white uppercase tracking-wider">Auto-download episodes</span>
                    <span class="text-[8px] text-black-100 font-medium leading-tight">Check for and download new episodes</span>
                  </div>
                  <label class="relative inline-flex items-center cursor-pointer flex-shrink-0">
                    <input type="checkbox" id="podcast-details-auto-download" class="sr-only peer" ${item.media.autoDownloadEpisodes ? 'checked' : ''}>
                    <div class="w-8 h-4 bg-black-400 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-accent"></div>
                  </label>
                </div>

                <!-- Auto-delete played episodes toggle -->
                <div class="flex items-center justify-between">
                  <div class="flex flex-col pr-2">
                    <span class="text-[10px] font-bold text-white uppercase tracking-wider">Auto-delete played episodes</span>
                    <span class="text-[8px] text-black-100 font-medium leading-tight">Remove files of episodes marked as played</span>
                  </div>
                  <label class="relative inline-flex items-center cursor-pointer flex-shrink-0">
                    <input type="checkbox" id="podcast-details-auto-delete-played" class="sr-only peer" ${item.media.autoDeletePlayed ? 'checked' : ''}>
                    <div class="w-8 h-4 bg-black-400 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-accent"></div>
                  </label>
                </div>

                <!-- Auto-download Schedule -->
                <div class="flex flex-col space-y-1">
                  <label for="podcast-details-schedule" class="text-[9px] font-bold text-black-50 uppercase tracking-wider">Download Schedule (Cron)</label>
                  <input type="text" id="podcast-details-schedule" class="bg-black-500 text-white border border-black-300 rounded px-2 py-1 text-xs focus:outline-none focus:border-accent" value="${item.media.autoDownloadSchedule || ''}" placeholder="e.g. 0 0 * * * (empty for default)">
                </div>

                <!-- Max episodes to keep -->
                <div class="grid grid-cols-2 gap-2">
                  <div class="flex flex-col space-y-1">
                    <label for="podcast-details-max-keep" class="text-[9px] font-bold text-black-50 uppercase tracking-wider">Episodes to Keep</label>
                    <input type="number" id="podcast-details-max-keep" class="bg-black-500 text-white border border-black-300 rounded px-2 py-1 text-xs focus:outline-none focus:border-accent" value="${item.media.maxEpisodesToKeep || 0}" min="0">
                  </div>
                  <div class="flex flex-col space-y-1">
                    <label for="podcast-details-max-new" class="text-[9px] font-bold text-black-50 uppercase tracking-wider">Max New Downloads</label>
                    <input type="number" id="podcast-details-max-new" class="bg-black-500 text-white border border-black-300 rounded px-2 py-1 text-xs focus:outline-none focus:border-accent" value="${item.media.maxNewEpisodesToDownload || 0}" min="0">
                  </div>
                </div>

                <!-- Skip intro and outro configurations -->
                <div class="grid grid-cols-2 gap-2">
                  <div class="flex flex-col space-y-1">
                    <label for="podcast-details-skip-intro" class="text-[9px] font-bold text-black-50 uppercase tracking-wider">Skip Intro (seconds)</label>
                    <input type="number" id="podcast-details-skip-intro" class="bg-black-500 text-white border border-black-300 rounded px-2 py-1 text-xs focus:outline-none focus:border-accent" value="${item.media.skipIntroDuration || 0}" min="0">
                  </div>
                  <div class="flex flex-col space-y-1">
                    <label for="podcast-details-skip-outro" class="text-[9px] font-bold text-black-50 uppercase tracking-wider">Skip Outro (seconds)</label>
                    <input type="number" id="podcast-details-skip-outro" class="bg-black-500 text-white border border-black-300 rounded px-2 py-1 text-xs focus:outline-none focus:border-accent" value="${item.media.skipOutroDuration || 0}" min="0">
                  </div>
                </div>

                <div class="flex justify-end pt-1">
                  <button id="podcast-details-save-settings-btn" class="bg-accent hover:bg-accent-hover text-primary text-[10px] font-bold uppercase tracking-wider px-3 py-1 rounded transition-colors focus:outline-none">
                    Save Settings
                  </button>
                </div>
              </div>
            ` : ''}

            ${mediaType === 'book' && item.media && item.media.tracks && item.media.tracks.length > 0 ? `
              <details class="group space-y-2 select-none" ${item.media.tracks.length <= 5 ? 'open' : ''}>
                <summary class="flex items-center justify-between border-b border-black-400 pb-1 cursor-pointer outline-none list-none">
                  <div class="flex items-center space-x-1">
                    <span class="material-symbols text-sm text-accent transition-transform group-open:rotate-90">chevron_right</span>
                    <span class="material-symbols text-sm text-accent">queue_music</span>
                    <span class="font-bold text-sm text-white font-medium">Audio Tracks (${item.media.tracks.length})</span>
                  </div>
                </summary>
                <div class="pt-2">
                  <ol class="space-y-1 max-h-64 overflow-y-auto no-scroll border border-black-400/50 rounded-md p-2 bg-primary/20 list-decimal list-inside text-xs">
                    ${item.media.tracks.map((t, idx) => `
                      <li class="p-2 hover:bg-black-500/40 rounded transition-colors text-black-50">
                        <span class="font-medium text-white pl-1">${escapeHtml(t.title)}</span>
                        <span class="float-right text-[0.7rem] text-black-100">${formatDuration(t.duration)}</span>
                      </li>
                    `).join('')}
                  </ol>
                </div>
              </details>
            ` : ''}

            ${mediaType === 'book' && item.media?.audioFiles && item.media.audioFiles.length > 0 ? `
              <details class="group space-y-2 select-none mt-4" ${item.media.audioFiles.length <= 5 ? 'open' : ''}>
                <summary class="flex items-center justify-between border-b border-black-400 pb-1 cursor-pointer outline-none list-none">
                  <div class="flex items-center space-x-1">
                    <span class="material-symbols text-sm text-accent transition-transform group-open:rotate-90">chevron_right</span>
                    <span class="material-symbols text-sm text-accent">audio_file</span>
                    <span class="font-bold text-sm text-white font-medium">Audio Files (${item.media.audioFiles.length})</span>
                  </div>
                </summary>
                <div class="pt-2">
                  <ol class="space-y-1 max-h-64 overflow-y-auto no-scroll border border-black-400/50 rounded-md p-2 bg-primary/20 list-decimal list-inside text-xs">
                    ${item.media.audioFiles.map((af, idx) => {
                      const filename = af.metadata?.filename || af.filename || `File ${idx + 1}`;
                      const durationStr = af.duration ? formatDuration(af.duration) : '';
                      const sizeStr = af.size ? formatBytes(af.size) : '';
                      return `
                        <li class="p-2 hover:bg-black-500/40 rounded transition-colors text-black-50 flex justify-between items-center">
                          <div class="flex flex-col min-w-0 pr-2">
                            <span class="font-medium text-white pl-1 truncate" title="${escapeHtml(filename)}">${escapeHtml(filename)}</span>
                            ${sizeStr ? `<span class="text-[10px] text-black-100 pl-1">${sizeStr}</span>` : ''}
                          </div>
                          ${durationStr ? `<span class="text-[0.7rem] text-black-100 flex-shrink-0">${durationStr}</span>` : ''}
                        </li>
                      `;
                    }).join('')}
                  </ol>
                </div>
              </details>
            ` : ''}

            ${mediaType === 'book' && hasAudio ? `
              <details class="group space-y-2 select-none mt-4" ${item.media?.chapters?.length > 0 && item.media.chapters.length <= 5 ? 'open' : ''}>
                <summary class="flex items-center justify-between border-b border-black-400 pb-1 cursor-pointer outline-none list-none">
                  <div class="flex items-center space-x-1">
                    <span class="material-symbols text-sm text-accent transition-transform group-open:rotate-90">chevron_right</span>
                    <span class="material-symbols text-sm text-accent">toc</span>
                    <span class="font-bold text-sm text-white font-medium">Chapters (${item.media?.chapters?.length || 0})</span>
                  </div>
                  <div class="flex items-center space-x-2">
                    ${isAdmin ? `
                      <button id="details-edit-chapters-btn" class="text-xs text-accent hover:underline flex items-center space-x-1" onclick="event.stopPropagation();">
                        <span class="material-symbols text-sm">edit</span>
                        <span>Edit Chapters</span>
                      </button>
                    ` : ''}
                  </div>
                </summary>
                <div class="pt-2">
                  ${item.media?.chapters?.length > 0 ? `
                    <ol class="space-y-1 max-h-64 overflow-y-auto no-scroll border border-black-400/50 rounded-md p-2 bg-primary/20 list-decimal list-inside text-xs">
                      ${item.media.chapters.map((c) => `
                        <li class="p-2 hover:bg-black-500/40 rounded transition-colors text-black-50 flex justify-between items-center cursor-pointer chapter-item-seek" data-start="${c.start}">
                          <span class="font-medium text-white pl-1">${escapeHtml(c.title)}</span>
                          <span class="float-right text-[0.7rem] text-black-100">${formatDuration(c.start)} - ${formatDuration(c.end)}</span>
                        </li>
                      `).join('')}
                    </ol>
                  ` : `
                    <p class="text-xs text-black-100">No chapters defined. Click "Edit Chapters" to create or lookup chapters.</p>
                  `}
                </div>
              </details>
            ` : ''}

            <!-- Bookmarks Section -->
            ${hasAudio ? `
              <div id="details-bookmarks-section" class="space-y-2 mt-4">
                <div class="flex items-center justify-between border-b border-black-400 pb-1">
                  <h3 class="font-bold text-sm text-white flex items-center space-x-1">
                    <span class="material-symbols text-sm text-accent">bookmarks</span>
                    <span>Bookmarks</span>
                  </h3>
                  <div class="flex items-center space-x-3">
                    <button id="details-add-bookmark-btn" class="text-xs text-accent hover:underline flex items-center space-x-1">
                      <span class="material-symbols text-sm">bookmark_add</span>
                      <span>Add Bookmark</span>
                    </button>
                    <button id="details-import-bookmarks-btn" class="text-xs text-accent hover:underline flex items-center space-x-1">
                      <span class="material-symbols text-sm">upload</span>
                      <span>Import</span>
                    </button>
                    <button id="details-export-bookmarks-btn" class="text-xs text-accent hover:underline flex items-center space-x-1">
                      <span class="material-symbols text-sm">download</span>
                      <span>Export</span>
                    </button>
                  </div>
                </div>
                <div id="bookmarks-list-container"></div>
              </div>
            ` : ''}

            <!-- Other Versions / Narrators Section -->
            ${item.otherVersions && item.otherVersions.length > 0 ? `
              <div id="details-other-versions-section" class="space-y-2 mt-4 text-left">
                <div class="flex items-center justify-between border-b border-black-400 pb-1">
                  <h3 class="font-bold text-sm text-white flex items-center space-x-1">
                    <span class="material-symbols text-sm text-accent font-bold">library_books</span>
                    <span>Other Versions / Narrators (${item.otherVersions.length})</span>
                  </h3>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2">
                  ${item.otherVersions.map(v => {
                    const token = localStorage.getItem('token');
                    const ts = Date.now();
                    const vCoverUrl = v.coverPath 
                      ? resolvePath(`/api/items/${v.id}/cover?token=${token}&ts=${ts}`)
                      : 'assets/images/logo.png';
                    const narratorsStr = v.narrators && v.narrators.length > 0 
                      ? v.narrators.join(', ') 
                      : 'Unknown Narrator';
                    const durationStr = formatDuration(v.duration);
                    return `
                      <div class="other-version-card flex items-center space-x-3 p-2.5 rounded-md border border-black-400/50 bg-primary/10 hover:bg-black-500/40 cursor-pointer transition-all duration-200" data-id="${v.id}">
                        <img src="${vCoverUrl}" class="w-10 h-14 object-cover rounded border border-black-400/60 flex-shrink-0" onerror="this.onerror=null; this.src='assets/images/logo.png'">
                        <div class="min-w-0 flex-grow text-xs text-left">
                          <p class="font-bold text-white truncate text-sm">${escapeHtml(v.title)}</p>
                          ${v.subtitle ? `<p class="text-black-100 truncate mt-0.5">${escapeHtml(v.subtitle)}</p>` : ''}
                          <p class="text-black-50 mt-1 flex items-center space-x-1">
                            <span class="material-symbols text-[11px] text-accent">record_voice_over</span>
                            <span class="truncate">${escapeHtml(narratorsStr)}</span>
                          </p>
                          <p class="text-black-100 mt-0.5 flex items-center space-x-1">
                            <span class="material-symbols text-[11px] text-black-100">schedule</span>
                            <span>${durationStr}</span>
                          </p>
                        </div>
                      </div>
                    `;
                  }).join('')}
                </div>
              </div>
            ` : ''}
          </div>
        </div>
      </div>
    `;

    // Hook click events
    document.getElementById('details-back-btn').onclick = backCallback;

    container.querySelectorAll('.other-version-card').forEach(card => {
      card.onclick = () => {
        loadItemDetails(card.dataset.id, libraryId, backCallback);
      };
    });

    container.querySelectorAll('.author-link').forEach(link => {
      link.onclick = (e) => {
        e.preventDefault();
        window.dispatchEvent(new CustomEvent('navigate-to-author', {
          detail: {
            authorId: link.dataset.id,
            authorName: link.dataset.name
          }
        }));
      };
    });

    container.querySelectorAll('.series-link').forEach(link => {
      link.onclick = (e) => {
        e.preventDefault();
        window.dispatchEvent(new CustomEvent('navigate-to-series', {
          detail: {
            seriesId: link.dataset.id,
            seriesName: link.dataset.name
          }
        }));
      };
    });

    container.querySelectorAll('.narrator-link').forEach(link => {
      link.onclick = (e) => {
        e.preventDefault();
        const encodedName = btoa(unescape(encodeURIComponent(link.dataset.name)));
        window.dispatchEvent(new CustomEvent('navigate-to-dashboard', {
          detail: {
            filterBy: `narrators.${encodedName}`,
            filterLabel: `Narrator: ${link.dataset.name}`
          }
        }));
      };
    });

    container.querySelectorAll('.genre-link').forEach(link => {
      link.onclick = (e) => {
        e.preventDefault();
        const encodedName = btoa(unescape(encodeURIComponent(link.dataset.name)));
        window.dispatchEvent(new CustomEvent('navigate-to-dashboard', {
          detail: {
            filterBy: `genres.${encodedName}`,
            filterLabel: `Genre: ${link.dataset.name}`
          }
        }));
      };
    });

    container.querySelectorAll('.tag-link').forEach(link => {
      link.onclick = (e) => {
        e.preventDefault();
        const encodedName = btoa(unescape(encodeURIComponent(link.dataset.name)));
        window.dispatchEvent(new CustomEvent('navigate-to-dashboard', {
          detail: {
            filterBy: `tags.${encodedName}`,
            filterLabel: `Tag: ${link.dataset.name}`
          }
        }));
      };
    });

    const shareBtn = document.getElementById('details-share-btn');
    if (shareBtn) {
      shareBtn.onclick = () => triggerShareLinkModal(item);
    }

    if (isAdmin) {
      const matchBtn = document.getElementById('details-match-btn');
      if (matchBtn) {
        matchBtn.onclick = () => triggerMatchBookModal(item, libraryId, () => loadItemDetails(itemId, libraryId, backCallback));
      }
      const matchCoverBtn = document.getElementById('details-match-cover-btn');
      if (matchCoverBtn) {
        matchCoverBtn.onclick = () => triggerCoverEditorModal(item, libraryId, () => loadItemDetails(itemId, libraryId, backCallback));
      }
      
      const coverContainer = document.getElementById('details-cover-container');
      if (coverContainer) {
        coverContainer.onclick = () => triggerCoverEditorModal(item, libraryId, () => loadItemDetails(itemId, libraryId, backCallback));
        
        coverContainer.ondragover = (e) => {
          e.preventDefault();
          coverContainer.classList.add('border-accent');
        };

        coverContainer.ondragleave = () => {
          coverContainer.classList.remove('border-accent');
        };

        coverContainer.ondrop = async (e) => {
          e.preventDefault();
          coverContainer.classList.remove('border-accent');
          const file = e.dataTransfer.files[0];
          if (file && file.type.startsWith('image/')) {
            const formData = new FormData();
            formData.append('cover', file, 'cover.jpg');

            try {
              const overlay = coverContainer.querySelector('div');
              if (overlay) {
                overlay.innerHTML = `
                  <span class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mb-2"></span>
                  <span class="text-xs text-white">Uploading...</span>
                `;
                overlay.style.opacity = 1;
              }

              const response = await fetch(resolvePath(`/api/items/${item.id}/cover`), {
                method: 'POST',
                headers: {
                  'Authorization': `Bearer ${localStorage.getItem('token') || ''}`
                },
                body: formData
              });

              if (!response.ok) {
                throw new Error(await response.text());
              }

              loadItemDetails(itemId, libraryId, backCallback);
            } catch (err) {
              alert('Failed to upload cover: ' + err.message);
              loadItemDetails(itemId, libraryId, backCallback);
            }
          }
        };
      }
      const embedMetadataBtn = document.getElementById('details-embed-metadata-btn');
      if (embedMetadataBtn) {
        embedMetadataBtn.onclick = async () => {
          if (!confirm('Are you sure you want to write and embed metadata, chapters, and cover art directly into the audio files? This will overwrite the tags of the files on disk.')) {
            return;
          }
          embedMetadataBtn.disabled = true;
          embedMetadataBtn.innerHTML = '<span class="material-symbols text-sm animate-spin">sync</span><span>Embedding...</span>';
          try {
            const resp = await request('POST', `/api/items/${item.id}/embed-metadata`);
            alert(resp.message || 'Metadata embedded successfully!');
          } catch (err) {
            alert('Failed to embed metadata: ' + (err.message || err));
          } finally {
            embedMetadataBtn.disabled = false;
            embedMetadataBtn.innerHTML = '<span class="material-symbols text-sm">settings_suggest</span><span>Embed Metadata</span>';
          }
        };
      }
      const mergeAudioBtn = document.getElementById('details-merge-audio-btn');
      if (mergeAudioBtn) {
        mergeAudioBtn.onclick = async () => {
          if (!confirm('Are you sure you want to merge all separate audio tracks into a single M4B file? This will merge the files, create chapters, update the database, and delete the original files.')) {
            return;
          }
          mergeAudioBtn.disabled = true;
          mergeAudioBtn.innerHTML = '<span class="material-symbols text-sm animate-spin">sync</span><span>Merging...</span>';
          try {
            const resp = await request('POST', `/api/items/${item.id}/merge`);
            alert(resp.message || 'Audio files merged successfully!');
            loadItemDetails(itemId, libraryId, backCallback);
          } catch (err) {
            alert('Failed to merge audio files: ' + (err.message || err));
          } finally {
            mergeAudioBtn.disabled = false;
            mergeAudioBtn.innerHTML = '<span class="material-symbols text-sm">call_merge</span><span>Merge Audio Files</span>';
          }
        };
      }
      document.getElementById('details-edit-btn').onclick = () => triggerEditItemDetailsModal(item, libraryId, () => loadItemDetails(itemId, libraryId, backCallback));
      const editChaptersBtn = document.getElementById('details-edit-chapters-btn');
      if (editChaptersBtn) {
        editChaptersBtn.onclick = () => triggerEditChaptersModal(item, () => loadItemDetails(itemId, libraryId, backCallback));
      }
    }

    if (hasAudio) {
      // Wire up chapter seeking on click
      container.querySelectorAll('.chapter-item-seek').forEach(li => {
        li.onclick = () => {
          const start = parseFloat(li.dataset.start);
          const currentPlaying = getCurrentPlayingItem();
          if (currentPlaying && currentPlaying.id === item.id) {
            seekTo(start);
          } else {
            playItem(item, start);
          }
        };
      });

      const playActionBtn = document.getElementById('details-play-action-btn');
      if (playActionBtn) {
        playActionBtn.onclick = async () => {
          try {
            let startTime = 0;
            try {
              const progressObj = await request('GET', `/api/me/progress/${item.id}`);
              if (progressObj && progressObj.currentTime !== undefined) {
                startTime = progressObj.currentTime;
              }
            } catch (err) {
              // Ignore progress fetch error, starts from 0
            }
            playItem(item, startTime);
          } catch (err) {
            console.error('Failed to start item playback:', err);
          }
        };
      }

      const queueActionBtn = document.getElementById('details-queue-action-btn');
      if (queueActionBtn) {
        queueActionBtn.onclick = () => {
          addToQueue(item);
        };
      }

      if (mediaType === 'podcast') {
        const episodesSection = document.getElementById('podcast-episodes-section');
        if (episodesSection) {
          (async () => {
            // 1. Fetch progresses
            let progresses = [];
            try {
              progresses = await request('GET', `/api/me/progress/${item.id}`);
            } catch (err) {
              console.warn('Failed to fetch podcast progresses:', err);
            }

            // Build a lookup map of progress
            const progressMap = {};
            if (Array.isArray(progresses)) {
              progresses.forEach(p => {
                if (p.episodeId) {
                  progressMap[p.episodeId] = p;
                }
              });
            }

            // Global/local variables to track search, filter, and sort state
            let searchQuery = '';
            let filterValue = 'all';
            let sortValue = 'newest';

            // Render the controls and the list wrapper
            episodesSection.innerHTML = `
              <div class="flex flex-col md:flex-row md:items-center justify-between gap-3 border-b border-black-400 pb-2">
                <div class="flex items-center space-x-2">
                  <h3 class="font-bold text-sm text-white">Episodes (<span id="episodes-count-badge">0</span>)</h3>
                  <button id="podcast-sync-feed-btn" class="flex items-center justify-center p-1.5 rounded-full hover:bg-black-400/50 text-accent transition-colors" title="Sync Feed">
                    <span class="material-symbols text-lg font-bold" id="podcast-sync-icon">sync</span>
                  </button>
                  <button id="podcast-multi-select-toggle-btn" class="flex items-center justify-center px-2 py-1 rounded hover:bg-black-400/50 text-black-50 hover:text-white transition-colors border border-black-400/50 text-[11px]" title="Toggle Multi-select">
                    <span class="material-symbols text-sm mr-1">checklist</span>
                    <span id="multi-select-toggle-text">Select</span>
                  </button>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <!-- Search input -->
                  <div class="relative">
                    <input type="text" id="episodes-search-input" placeholder="Search episodes..." class="bg-black-500 text-white border border-black-300 rounded pl-8 pr-2.5 py-1 text-xs w-48 focus:outline-none focus:border-accent">
                    <span class="material-symbols text-xs text-black-100 absolute left-2.5 top-[8px]">search</span>
                  </div>
                  <!-- Filter Select -->
                  <select id="episodes-filter-select" class="bg-black-500 text-white border border-black-300 rounded px-2.5 py-1 text-xs focus:outline-none focus:border-accent cursor-pointer">
                    <option value="all">All Episodes</option>
                    <option value="downloaded">Downloaded</option>
                    <option value="not-downloaded">Not Downloaded</option>
                    <option value="in-progress">In Progress</option>
                    <option value="played">Played</option>
                    <option value="unplayed">Unplayed</option>
                  </select>
                  <!-- Sort Select -->
                  <select id="episodes-sort-select" class="bg-black-500 text-white border border-black-300 rounded px-2.5 py-1 text-xs focus:outline-none focus:border-accent cursor-pointer">
                    <option value="newest">Newest First</option>
                    <option value="oldest">Oldest First</option>
                  </select>
                </div>
              </div>
              <!-- Episode list container -->
              <ul class="space-y-2.5 max-h-[500px] overflow-y-auto no-scroll border border-black-400/50 rounded-md p-3 bg-primary/20" id="podcast-episodes-list">
              </ul>

              <!-- Batch Actions Toolbar -->
              <div id="podcast-batch-actions-toolbar" class="hidden sticky bottom-0 bg-primary border-t border-black-400 p-3 mt-3 flex flex-wrap items-center justify-between gap-2 rounded-b-md shadow-lg z-30">
                <div class="flex items-center space-x-2">
                  <input type="checkbox" id="podcast-batch-select-all" class="w-4 h-4 rounded border-black-300 text-accent focus:ring-accent cursor-pointer">
                  <label for="podcast-batch-select-all" class="text-xs text-white cursor-pointer select-none">Select All (<span id="batch-selected-count">0</span>)</label>
                </div>
                <div class="flex items-center space-x-2">
                  <button id="batch-download-btn" class="bg-accent text-primary text-xs font-bold px-3 py-1.5 rounded hover:opacity-90 transition-opacity flex items-center space-x-1">
                    <span class="material-symbols text-sm">download</span>
                    <span>Download</span>
                  </button>
                  <button id="batch-played-btn" class="bg-black-400 hover:bg-black-300 text-white text-xs font-bold px-3 py-1.5 rounded transition-colors flex items-center space-x-1">
                    <span class="material-symbols text-sm">check_circle</span>
                    <span>Mark Played</span>
                  </button>
                  <button id="batch-unplayed-btn" class="bg-black-400 hover:bg-black-300 text-white text-xs font-bold px-3 py-1.5 rounded transition-colors flex items-center space-x-1">
                    <span class="material-symbols text-sm">radio_button_unchecked</span>
                    <span>Mark Unplayed</span>
                  </button>
                  <button id="batch-delete-btn" class="bg-red-600/20 hover:bg-red-600/30 border border-red-500/30 text-red-400 text-xs font-bold px-3 py-1.5 rounded transition-colors flex items-center space-x-1">
                    <span class="material-symbols text-sm">delete</span>
                    <span>Delete</span>
                  </button>
                </div>
              </div>
            `;

            // Reference DOM elements
            const episodesCountBadge = document.getElementById('episodes-count-badge');
            const episodesList = document.getElementById('podcast-episodes-list');
            const searchInput = document.getElementById('episodes-search-input');
            const filterSelect = document.getElementById('episodes-filter-select');
            const sortSelect = document.getElementById('episodes-sort-select');
            const syncFeedBtn = document.getElementById('podcast-sync-feed-btn');
            const syncIcon = document.getElementById('podcast-sync-icon');
            const multiSelectToggleBtn = document.getElementById('podcast-multi-select-toggle-btn');
            const multiSelectToggleText = document.getElementById('multi-select-toggle-text');

            // Expanded items state
            const expandedEpisodes = new Set();

            // Multi-select state
            let multiSelectMode = false;
            const selectedEpisodes = new Set();

            // Render episodes list
            const renderList = () => {
              let episodes = [...(item.media.episodes || [])];

              // Filter episodes
              if (searchQuery) {
                const query = searchQuery.toLowerCase();
                episodes = episodes.filter(ep => 
                  (ep.title && ep.title.toLowerCase().includes(query)) ||
                  (ep.description && ep.description.toLowerCase().includes(query))
                );
              }

              if (filterValue !== 'all') {
                episodes = episodes.filter(ep => {
                  const isDownloaded = ep.audioFile && ep.audioFile.metadata && ep.audioFile.metadata.path;
                  const prog = progressMap[ep.id];
                  const isPlayed = prog && prog.isFinished;
                  const isInProgress = prog && !prog.isFinished && prog.currentTime > 0;
                  
                  if (filterValue === 'downloaded') return isDownloaded;
                  if (filterValue === 'not-downloaded') return !isDownloaded;
                  if (filterValue === 'in-progress') return isInProgress;
                  if (filterValue === 'played') return isPlayed;
                  if (filterValue === 'unplayed') return !isPlayed && !isInProgress;
                  return true;
                });
              }

              // Sort episodes
              episodes.sort((a, b) => {
                const dateA = a.pubDate ? new Date(a.pubDate) : new Date(0);
                const dateB = b.pubDate ? new Date(b.pubDate) : new Date(0);
                return sortValue === 'newest' ? dateB - dateA : dateA - dateB;
              });

              // Update badge count
              episodesCountBadge.textContent = episodes.length;

              if (episodes.length === 0) {
                episodesList.innerHTML = `
                  <div class="text-center py-8 text-black-100">
                    <span class="material-symbols text-3xl">info</span>
                    <p class="mt-2 text-xs">No episodes found matching criteria.</p>
                  </div>
                `;
                return;
              }

              // Update batch actions toolbar visibility and checkboxes
              const toolbar = document.getElementById('podcast-batch-actions-toolbar');
              if (toolbar) {
                if (multiSelectMode) {
                  toolbar.classList.remove('hidden');
                } else {
                  toolbar.classList.add('hidden');
                }
              }

              const selectedCountBadge = document.getElementById('batch-selected-count');
              if (selectedCountBadge) {
                selectedCountBadge.textContent = selectedEpisodes.size;
              }

              const selectAllCheckbox = document.getElementById('podcast-batch-select-all');
              if (selectAllCheckbox) {
                const allSelected = episodes.length > 0 && episodes.every(ep => selectedEpisodes.has(ep.id));
                selectAllCheckbox.checked = allSelected;
              }

              episodesList.innerHTML = episodes.map((ep) => {
                const isDownloaded = ep.audioFile && ep.audioFile.metadata && ep.audioFile.metadata.path;
                const fileSize = ep.audioFile && ep.audioFile.metadata && ep.audioFile.metadata.size;
                const durationVal = ep.duration || (ep.audioFile && ep.audioFile.duration) || 0;
                
                const prog = progressMap[ep.id];
                const isPlayed = prog && prog.isFinished;
                const isInProgress = prog && !prog.isFinished && prog.currentTime > 0;

                // Badges
                let statusBadge = '';
                if (isPlayed) {
                  statusBadge = `<span class="bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 text-[9px] font-bold px-1.5 py-0.5 rounded flex items-center gap-0.5"><span class="material-symbols text-[10px]">check</span>Played</span>`;
                } else if (isInProgress) {
                  const percent = Math.round(prog.progress * 100);
                  statusBadge = `<span class="bg-blue-500/10 border border-blue-500/30 text-blue-400 text-[9px] font-bold px-1.5 py-0.5 rounded flex items-center gap-1"><span class="animate-pulse w-1.5 h-1.5 bg-blue-400 rounded-full"></span>${percent}%</span>`;
                }

                let downloadBadge = '';
                if (isDownloaded) {
                  const sizeStr = fileSize ? ` (${formatBytes(fileSize)})` : '';
                  downloadBadge = `<span class="bg-indigo-500/10 border border-indigo-500/30 text-indigo-400 text-[9px] font-bold px-1.5 py-0.5 rounded flex items-center gap-0.5" title="Downloaded${sizeStr}"><span class="material-symbols text-[10px]">download_done</span>Downloaded</span>`;
                }

                // Play Button
                let playBtn = '';
                if (isDownloaded) {
                  let btnText = 'Play';
                  let btnIcon = 'play_arrow';
                  if (isInProgress) {
                    btnText = 'Resume';
                    btnIcon = 'play_arrow';
                  } else if (isPlayed) {
                    btnText = 'Replay';
                    btnIcon = 'replay';
                  }
                  playBtn = `
                    <button class="episode-play-btn flex items-center space-x-1 bg-accent text-primary px-2.5 py-1 rounded font-bold hover:opacity-90 transition-opacity" data-id="${ep.id}">
                      <span class="material-symbols text-sm font-bold">${btnIcon}</span>
                      <span>${btnText}</span>
                    </button>
                  `;
                }

                // Download Action Button
                let downloadActionBtn = '';
                if (!isDownloaded) {
                  downloadActionBtn = `
                    <button class="episode-download-btn flex items-center space-x-1 bg-black-400 hover:bg-black-300 border border-black-300 text-white px-2.5 py-1 rounded font-bold transition-colors" data-id="${ep.id}">
                      <span class="material-symbols text-sm">download</span>
                      <span>Download</span>
                    </button>
                  `;
                }

                const isExpanded = expandedEpisodes.has(ep.id);

                // Episode-specific cover art or fallback
                const imageUrl = ep.imageUrl || ep.imageURL;
                const coverArtHtml = imageUrl
                  ? `<img src="${escapeHtml(imageUrl)}" class="w-10 h-10 rounded border border-black-400 object-cover flex-shrink-0 mr-3" alt="">`
                  : `<div class="w-10 h-10 bg-black-500 rounded border border-black-400 flex items-center justify-center flex-shrink-0 mr-3 text-black-200"><span class="material-symbols text-lg">podcasts</span></div>`;

                // Season/Episode numbers string
                let metaString = '';
                if (ep.season) {
                  metaString += `Season ${escapeHtml(ep.season)} `;
                }
                if (ep.episode) {
                  metaString += `Episode ${escapeHtml(ep.episode)}`;
                }
                metaString = metaString.trim();

                // Multi-select checkbox HTML
                let checkboxHtml = '';
                if (multiSelectMode) {
                  const isChecked = selectedEpisodes.has(ep.id);
                  checkboxHtml = `
                    <input type="checkbox" class="episode-select-checkbox w-4 h-4 rounded border-black-300 text-accent focus:ring-accent mr-3 cursor-pointer" data-id="${ep.id}" ${isChecked ? 'checked' : ''} onclick="event.stopPropagation()">
                  `;
                }

                return `
                  <li class="border border-black-400/30 hover:border-black-400/70 bg-black-500/10 hover:bg-black-500/30 rounded-md transition-all text-xs" data-episode-id="${ep.id}">
                    <!-- Header row (clickable to expand description) -->
                    <div class="flex items-start justify-between p-3 cursor-pointer select-none" data-action="toggle-expand" data-id="${ep.id}">
                      <div class="flex items-center flex-grow mr-4 min-w-0">
                        ${checkboxHtml}
                        ${coverArtHtml}
                        <div class="min-w-0 flex-grow">
                          <div class="flex items-center space-x-2">
                            <span class="font-bold text-white text-xs hover:text-accent transition-colors truncate block max-w-md">${escapeHtml(ep.title)}</span>
                            <!-- Badges -->
                            <div class="flex items-center space-x-1.5 flex-shrink-0">
                              ${statusBadge}
                              ${downloadBadge}
                            </div>
                          </div>
                          <div class="flex items-center space-x-3 text-[10px] text-black-100 mt-1">
                            ${ep.pubDate ? `<span>${escapeHtml(ep.pubDate)}</span>` : ''}
                            ${durationVal > 0 ? `<span>•</span><span>${formatDuration(durationVal)}</span>` : ''}
                            ${metaString ? `<span>•</span><span class="text-accent font-medium">${metaString}</span>` : ''}
                          </div>
                        </div>
                      </div>
                      <!-- Play & Actions Buttons -->
                      <div class="flex items-center space-x-2 flex-shrink-0" onclick="event.stopPropagation()">
                        ${playBtn}
                        ${downloadActionBtn}
                        <!-- Episode Option Button / Dropdown -->
                        <div class="relative inline-block text-left">
                          <button class="episode-actions-trigger p-1 hover:bg-black-400 rounded transition-colors text-black-100 hover:text-white" data-id="${ep.id}">
                            <span class="material-symbols text-sm font-bold">more_vert</span>
                          </button>
                          <!-- Dropdown menu -->
                          <div class="episode-actions-dropdown hidden absolute right-0 mt-1 w-36 rounded-md shadow-lg bg-black-500 border border-black-300 ring-1 ring-black ring-opacity-5 z-20">
                            <div class="py-1" role="menu">
                              <button class="mark-played-btn flex w-full items-center px-3 py-2 text-left hover:bg-black-400 text-white" data-id="${ep.id}">
                                <span class="material-symbols text-xs mr-2">check_circle</span>
                                <span>Mark Played</span>
                              </button>
                              <button class="mark-unplayed-btn flex w-full items-center px-3 py-2 text-left hover:bg-black-400 text-white" data-id="${ep.id}">
                                <span class="material-symbols text-xs mr-2">radio_button_unchecked</span>
                                <span>Mark Unplayed</span>
                              </button>
                              ${isDownloaded ? `
                                <button class="delete-file-btn flex w-full items-center px-3 py-2 text-left hover:bg-black-400 text-red-400" data-id="${ep.id}">
                                  <span class="material-symbols text-xs mr-2">delete</span>
                                  <span>Delete File</span>
                                </button>
                              ` : ''}
                              <button class="hard-delete-btn flex w-full items-center px-3 py-2 text-left hover:bg-black-400 text-red-500 font-bold" data-id="${ep.id}">
                                <span class="material-symbols text-xs mr-2">delete_forever</span>
                                <span>Hard Delete</span>
                              </button>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                    <!-- Description Accordion panel (hidden by default) -->
                    <div class="episode-description ${isExpanded ? '' : 'hidden'} border-t border-black-400/20 p-3.5 bg-black-500/20 text-black-100 text-xs leading-relaxed max-h-48 overflow-y-auto no-scroll">
                      ${ep.description ? ep.description : '<em class="text-black-200">No description available.</em>'}
                    </div>
                  </li>
                `;
              }).join('');

              // Hook checkbox state changes
              episodesList.querySelectorAll('.episode-select-checkbox').forEach(chk => {
                chk.onchange = (e) => {
                  const epId = chk.getAttribute('data-id');
                  if (e.target.checked) {
                    selectedEpisodes.add(epId);
                  } else {
                    selectedEpisodes.delete(epId);
                  }
                  // Update select-all check state and count
                  const selectedCountBadge = document.getElementById('batch-selected-count');
                  if (selectedCountBadge) {
                    selectedCountBadge.textContent = selectedEpisodes.size;
                  }
                  const selectAllCheckbox = document.getElementById('podcast-batch-select-all');
                  if (selectAllCheckbox) {
                    const allSelected = episodes.length > 0 && episodes.every(ep => selectedEpisodes.has(ep.id));
                    selectAllCheckbox.checked = allSelected;
                  }
                };
              });

              // Hook play buttons
              episodesList.querySelectorAll('.episode-play-btn').forEach(btn => {
                btn.onclick = async () => {
                  const epId = btn.getAttribute('data-id');
                  const ep = item.media.episodes.find(e => e.id === epId);
                  if (!ep) return;

                  let startTime = 0;
                  const prog = progressMap[epId];
                  if (prog && prog.currentTime !== undefined) {
                    startTime = prog.currentTime;
                  }

                  const mockItem = {
                    ...item,
                    episodeId: ep.id,
                    media: {
                      ...item.media,
                      audioFiles: [ep.audioFile],
                      duration: ep.duration || 0,
                      metadata: {
                        ...item.media.metadata,
                        title: ep.title
                      }
                    }
                  };
                  playItem(mockItem, startTime);
                };
              });

              // Hook download buttons
              episodesList.querySelectorAll('.episode-download-btn').forEach(btn => {
                btn.onclick = async () => {
                  const epId = btn.getAttribute('data-id');
                  const originalContent = btn.innerHTML;
                  btn.disabled = true;
                  btn.innerHTML = `<span class="animate-spin text-white material-symbols text-xs">sync</span><span>Downloading...</span>`;
                  try {
                    await request('POST', `/api/podcasts/${item.mediaId || item.media.id}/download-episodes`, [epId]);
                    showToast('Download started in background', 'success');
                    btn.className = "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 text-[10px] font-bold px-2.5 py-1 rounded cursor-default focus:outline-none";
                    btn.innerHTML = `<span class="material-symbols text-xs">sync_saved_locally</span><span>Queued</span>`;
                    btn.onclick = null;
                  } catch (err) {
                    console.error('Download failed:', err);
                    showToast('Failed to start download: ' + err.message, 'error');
                    btn.disabled = false;
                    btn.innerHTML = originalContent;
                  }
                };
              });

              // Hook actions trigger dropdowns
              episodesList.querySelectorAll('.episode-actions-trigger').forEach(trigger => {
                trigger.onclick = (e) => {
                  e.stopPropagation();
                  const dropdown = trigger.nextElementSibling;
                  document.querySelectorAll('.episode-actions-dropdown').forEach(d => {
                    if (d !== dropdown) d.classList.add('hidden');
                  });
                  dropdown.classList.toggle('hidden');
                };
              });

              // Hook mark played buttons
              episodesList.querySelectorAll('.mark-played-btn').forEach(btn => {
                btn.onclick = async (e) => {
                  e.stopPropagation();
                  const epId = btn.getAttribute('data-id');
                  const ep = item.media.episodes.find(e => e.id === epId);
                  if (!ep) return;
                  const durationVal = ep.duration || (ep.audioFile && ep.audioFile.duration) || 0;

                  try {
                    const payload = {
                      currentTime: durationVal,
                      duration: durationVal,
                      isFinished: true
                    };
                    await request('PATCH', `/api/me/progress/${item.id}/${epId}`, payload);
                    showToast('Episode marked as played', 'success');
                    
                    progressMap[epId] = {
                      episodeId: epId,
                      currentTime: durationVal,
                      duration: durationVal,
                      isFinished: true,
                      progress: 1.0
                    };
                    renderList();
                  } catch (err) {
                    showToast('Failed to mark played: ' + err.message, 'error');
                  }
                };
              });

              // Hook mark unplayed buttons
              episodesList.querySelectorAll('.mark-unplayed-btn').forEach(btn => {
                btn.onclick = async (e) => {
                  e.stopPropagation();
                  const epId = btn.getAttribute('data-id');

                  try {
                    await request('DELETE', `/api/me/progress/${item.id}/${epId}`);
                    showToast('Episode marked as unplayed', 'success');
                    
                    delete progressMap[epId];
                    renderList();
                  } catch (err) {
                    showToast('Failed to mark unplayed: ' + err.message, 'error');
                  }
                };
              });

              // Hook delete downloaded file buttons
              episodesList.querySelectorAll('.delete-file-btn').forEach(btn => {
                btn.onclick = async (e) => {
                  e.stopPropagation();
                  if (!confirm('Are you sure you want to delete the local file for this episode?')) return;
                  const epId = btn.getAttribute('data-id');

                  try {
                    await request('DELETE', `/api/podcasts/${item.mediaId || item.media.id}/episode/${epId}`);
                    showToast('File deleted successfully', 'success');
                    
                    const ep = item.media.episodes.find(e => e.id === epId);
                    if (ep) {
                      ep.audioFile = {};
                    }
                    renderList();
                  } catch (err) {
                    showToast('Failed to delete file: ' + err.message, 'error');
                  }
                };
              });

              // Hook hard delete episode buttons
              episodesList.querySelectorAll('.hard-delete-btn').forEach(btn => {
                btn.onclick = async (e) => {
                  e.stopPropagation();
                  if (!confirm('Are you sure you want to completely delete this episode from the database? This cannot be undone.')) return;
                  const epId = btn.getAttribute('data-id');

                  try {
                    await request('DELETE', `/api/podcasts/${item.mediaId || item.media.id}/episode/${epId}?hard=1`);
                    showToast('Episode deleted permanently', 'success');
                    
                    item.media.episodes = item.media.episodes.filter(e => e.id !== epId);
                    renderList();
                  } catch (err) {
                    showToast('Failed to delete episode: ' + err.message, 'error');
                  }
                };
              });

              // Hook toggling expand description
              episodesList.querySelectorAll('[data-action="toggle-expand"]').forEach(header => {
                header.onclick = () => {
                  const epId = header.getAttribute('data-id');
                  if (expandedEpisodes.has(epId)) {
                    expandedEpisodes.delete(epId);
                  } else {
                    expandedEpisodes.add(epId);
                  }
                  renderList();
                };
              });
            };

            // Toggle multi-select mode
            if (multiSelectToggleBtn) {
              multiSelectToggleBtn.onclick = () => {
                multiSelectMode = !multiSelectMode;
                if (multiSelectMode) {
                  multiSelectToggleBtn.classList.add('bg-accent', 'text-primary');
                  multiSelectToggleBtn.classList.remove('text-black-50');
                  multiSelectToggleText.textContent = 'Cancel';
                } else {
                  multiSelectToggleBtn.classList.remove('bg-accent', 'text-primary');
                  multiSelectToggleBtn.classList.add('text-black-50');
                  multiSelectToggleText.textContent = 'Select';
                  selectedEpisodes.clear();
                }
                renderList();
              };
            }

            // Batch Select All
            const selectAllCheckbox = document.getElementById('podcast-batch-select-all');
            if (selectAllCheckbox) {
              selectAllCheckbox.onclick = (e) => {
                const episodes = [...(item.media.episodes || [])];
                if (e.target.checked) {
                  episodes.forEach(ep => selectedEpisodes.add(ep.id));
                } else {
                  selectedEpisodes.clear();
                }
                renderList();
              };
            }

            // Batch Download Action
            const batchDownloadBtn = document.getElementById('batch-download-btn');
            if (batchDownloadBtn) {
              batchDownloadBtn.onclick = async () => {
                if (selectedEpisodes.size === 0) return;
                const ids = Array.from(selectedEpisodes);
                batchDownloadBtn.disabled = true;
                try {
                  await request('POST', `/api/podcasts/${item.mediaId || item.media.id}/download-episodes`, ids);
                  showToast(`Queueing download for ${ids.length} episodes`, 'success');
                  selectedEpisodes.clear();
                  multiSelectMode = false;
                  if (multiSelectToggleBtn) {
                    multiSelectToggleBtn.classList.remove('bg-accent', 'text-primary');
                    multiSelectToggleBtn.classList.add('text-black-50');
                    multiSelectToggleText.textContent = 'Select';
                  }
                  renderList();
                } catch (err) {
                  showToast('Failed to queue downloads: ' + err.message, 'error');
                } finally {
                  batchDownloadBtn.disabled = false;
                }
              };
            }

            // Batch Mark Played Action
            const batchPlayedBtn = document.getElementById('batch-played-btn');
            if (batchPlayedBtn) {
              batchPlayedBtn.onclick = async () => {
                if (selectedEpisodes.size === 0) return;
                const ids = Array.from(selectedEpisodes);
                batchPlayedBtn.disabled = true;
                try {
                  let successCount = 0;
                  await Promise.all(ids.map(async (epId) => {
                    const ep = item.media.episodes.find(e => e.id === epId);
                    if (!ep) return;
                    const durationVal = ep.duration || (ep.audioFile && ep.audioFile.duration) || 0;
                    const payload = {
                      currentTime: durationVal,
                      duration: durationVal,
                      isFinished: true
                    };
                    await request('PATCH', `/api/me/progress/${item.id}/${epId}`, payload);
                    progressMap[epId] = {
                      episodeId: epId,
                      currentTime: durationVal,
                      duration: durationVal,
                      isFinished: true,
                      progress: 1.0
                    };
                    successCount++;
                  }));
                  showToast(`Marked ${successCount} episodes as played`, 'success');
                  selectedEpisodes.clear();
                  multiSelectMode = false;
                  if (multiSelectToggleBtn) {
                    multiSelectToggleBtn.classList.remove('bg-accent', 'text-primary');
                    multiSelectToggleBtn.classList.add('text-black-50');
                    multiSelectToggleText.textContent = 'Select';
                  }
                  renderList();
                } catch (err) {
                  showToast('Failed to mark episodes as played: ' + err.message, 'error');
                } finally {
                  batchPlayedBtn.disabled = false;
                }
              };
            }

            // Batch Mark Unplayed Action
            const batchUnplayedBtn = document.getElementById('batch-unplayed-btn');
            if (batchUnplayedBtn) {
              batchUnplayedBtn.onclick = async () => {
                if (selectedEpisodes.size === 0) return;
                const ids = Array.from(selectedEpisodes);
                batchUnplayedBtn.disabled = true;
                try {
                  let successCount = 0;
                  await Promise.all(ids.map(async (epId) => {
                    await request('DELETE', `/api/me/progress/${item.id}/${epId}`);
                    delete progressMap[epId];
                    successCount++;
                  }));
                  showToast(`Marked ${successCount} episodes as unplayed`, 'success');
                  selectedEpisodes.clear();
                  multiSelectMode = false;
                  if (multiSelectToggleBtn) {
                    multiSelectToggleBtn.classList.remove('bg-accent', 'text-primary');
                    multiSelectToggleBtn.classList.add('text-black-50');
                    multiSelectToggleText.textContent = 'Select';
                  }
                  renderList();
                } catch (err) {
                  showToast('Failed to mark episodes as unplayed: ' + err.message, 'error');
                } finally {
                  batchUnplayedBtn.disabled = false;
                }
              };
            }

            // Batch Delete Action (removes downloaded files)
            const batchDeleteBtn = document.getElementById('batch-delete-btn');
            if (batchDeleteBtn) {
              batchDeleteBtn.onclick = async () => {
                if (selectedEpisodes.size === 0) return;
                if (!confirm(`Are you sure you want to delete local files for the ${selectedEpisodes.size} selected episodes?`)) return;
                const ids = Array.from(selectedEpisodes);
                batchDeleteBtn.disabled = true;
                try {
                  let successCount = 0;
                  await Promise.all(ids.map(async (epId) => {
                    const ep = item.media.episodes.find(e => e.id === epId);
                    const isDownloaded = ep && ep.audioFile && ep.audioFile.metadata && ep.audioFile.metadata.path;
                    if (!isDownloaded) return;
                    await request('DELETE', `/api/podcasts/${item.mediaId || item.media.id}/episode/${epId}`);
                    if (ep) {
                      ep.audioFile = {};
                    }
                    successCount++;
                  }));
                  showToast(`Deleted files for ${successCount} episodes`, 'success');
                  selectedEpisodes.clear();
                  multiSelectMode = false;
                  if (multiSelectToggleBtn) {
                    multiSelectToggleBtn.classList.remove('bg-accent', 'text-primary');
                    multiSelectToggleBtn.classList.add('text-black-50');
                    multiSelectToggleText.textContent = 'Select';
                  }
                  renderList();
                } catch (err) {
                  showToast('Failed to delete episodes: ' + err.message, 'error');
                } finally {
                  batchDeleteBtn.disabled = false;
                }
              };
            }

            // Hook filter and sort inputs
            searchInput.oninput = (e) => {
              searchQuery = e.target.value;
              renderList();
            };

            filterSelect.onchange = (e) => {
              filterValue = e.target.value;
              renderList();
            };

            sortSelect.onchange = (e) => {
              sortValue = e.target.value;
              renderList();
            };

            // Close dropdowns on document click
            document.addEventListener('click', () => {
              document.querySelectorAll('.episode-actions-dropdown').forEach(d => d.classList.add('hidden'));
            });

            // Sync feed handler
            syncFeedBtn.onclick = async () => {
              syncFeedBtn.disabled = true;
              syncIcon.classList.add('animate-spin');
              try {
                const res = await request('GET', `/api/podcasts/${item.mediaId || item.media.id}/checknew`);
                if (res && res.episodes) {
                  item.media.episodes = res.episodes;
                  showToast('Podcast feed synced successfully', 'success');
                } else {
                  showToast('Podcast feed synced', 'success');
                }
                renderList();
              } catch (err) {
                console.error('Failed to sync feed:', err);
                showToast('Sync feed failed: ' + err.message, 'error');
              } finally {
                syncFeedBtn.disabled = false;
                syncIcon.classList.remove('animate-spin');
              }
            };

            // Initial render
            renderList();
          })();
        }
      }

      // Hook podcast settings save
      const saveSettingsBtn = document.getElementById('podcast-details-save-settings-btn');
      if (saveSettingsBtn) {
        saveSettingsBtn.onclick = async () => {
          const autoDownload = document.getElementById('podcast-details-auto-download').checked;
          const autoDeletePlayed = document.getElementById('podcast-details-auto-delete-played').checked;
          const autoDownloadSchedule = document.getElementById('podcast-details-schedule').value.trim();
          const maxKeep = parseInt(document.getElementById('podcast-details-max-keep').value, 10) || 0;
          const maxNew = parseInt(document.getElementById('podcast-details-max-new').value, 10) || 0;
          
          saveSettingsBtn.disabled = true;
          const originalText = saveSettingsBtn.textContent;
          saveSettingsBtn.innerHTML = `<span class="animate-spin text-primary material-symbols text-xs">sync</span>`;

          try {
            const skipIntro = parseInt(document.getElementById('podcast-details-skip-intro').value, 10) || 0;
            const skipOutro = parseInt(document.getElementById('podcast-details-skip-outro').value, 10) || 0;
            
            await request('PATCH', `/api/items/${item.id}`, {
              title: item.media.metadata.title,
              autoDownloadEpisodes: autoDownload,
              autoDeletePlayed: autoDeletePlayed,
              autoDownloadSchedule: autoDownloadSchedule,
              maxEpisodesToKeep: maxKeep,
              maxNewEpisodesToDownload: maxNew,
              skipIntroDuration: skipIntro,
              skipOutroDuration: skipOutro
            });
            showToast('Podcast settings saved successfully', 'success');
            // Update local state
            item.media.autoDownloadEpisodes = autoDownload;
            item.media.autoDeletePlayed = autoDeletePlayed;
            item.media.autoDownloadSchedule = autoDownloadSchedule;
            item.media.maxEpisodesToKeep = maxKeep;
            item.media.maxNewEpisodesToDownload = maxNew;
            item.media.skipIntroDuration = skipIntro;
            item.media.skipOutroDuration = skipOutro;
          } catch (err) {
            console.error('Failed to save podcast settings:', err);
            showToast('Failed to save settings: ' + err.message, 'error');
          } finally {
            saveSettingsBtn.disabled = false;
            saveSettingsBtn.textContent = originalText;
          }
        };
      }
    }

    if (hasEbook) {
      const readActionBtn = document.getElementById('details-read-action-btn');
      if (readActionBtn) {
        readActionBtn.onclick = () => {
          openEbookReader(item, token);
        };
      }

      const sendDeviceBtn = document.getElementById('details-send-device-btn');
      if (sendDeviceBtn) {
        sendDeviceBtn.onclick = async () => {
          try {
            // Fetch available devices
            const devices = await request('GET', '/api/emails/devices');
            if (!devices || devices.length === 0) {
              alert('No e-reader devices are currently configured or available for your account.');
              return;
            }

            // Create a selection modal/popup
            const modal = document.createElement('div');
            modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4';
            modal.innerHTML = `
              <div class="bg-primary border border-black-300 w-full max-w-sm rounded-md shadow-2xl p-6 relative text-left">
                <h3 class="text-md font-bold text-white mb-3 border-b border-black-400 pb-2">Send E-Book to Device</h3>
                
                <div class="space-y-4">
                  <div>
                    <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Select Target Device</label>
                    <select id="send-target-device" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
                      ${devices.map(dev => `<option value="${escapeHtml(dev.name)}">${escapeHtml(dev.name)} (${escapeHtml(dev.email)})</option>`).join('')}
                    </select>
                  </div>

                  <div class="flex justify-end space-x-3 pt-2">
                    <button type="button" id="close-send-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold">Cancel</button>
                    <button type="button" id="confirm-send-device-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">Send</button>
                  </div>
                </div>
              </div>
            `;

            document.body.appendChild(modal);

            const closeModal = () => modal.remove();
            modal.querySelector('#close-send-modal-btn').onclick = closeModal;

            modal.querySelector('#confirm-send-device-btn').onclick = async () => {
              const selectedDevice = modal.querySelector('#send-target-device').value;
              const sendBtn = modal.querySelector('#confirm-send-device-btn');
              sendBtn.textContent = 'Sending...';
              sendBtn.disabled = true;

              try {
                await request('POST', '/api/emails/send-ebook-to-device', {
                  libraryItemId: item.id,
                  deviceName: selectedDevice
                });
                alert(`E-Book successfully queued to send to "${selectedDevice}"!`);
                closeModal();
              } catch (err) {
                alert('Failed to send e-book: ' + err.message);
                sendBtn.textContent = 'Send';
                sendBtn.disabled = false;
              }
            };

          } catch (err) {
            alert('Error loading devices: ' + err.message);
          }
        };
      }
    }

    if (hasAudio) {
      // Load bookmarks
      renderBookmarks(item);

      // Hook up add bookmark button
      const addBookmarkBtn = document.getElementById('details-add-bookmark-btn');
      if (addBookmarkBtn) {
        addBookmarkBtn.onclick = () => {
          triggerAddBookmarkOnDetailsModal(item);
        };
      }

      // Hook up import bookmarks button
      const importBookmarksBtn = document.getElementById('details-import-bookmarks-btn');
      if (importBookmarksBtn) {
        importBookmarksBtn.onclick = () => {
          triggerImportBookmarksModal(item);
        };
      }

      // Hook up export bookmarks button
      const exportBookmarksBtn = document.getElementById('details-export-bookmarks-btn');
      if (exportBookmarksBtn) {
        exportBookmarksBtn.onclick = () => {
          triggerExportBookmarksModal(item);
        };
      }

      // Hook up global bookmark-added listener for updates
      const onBookmarkUpdate = (e) => {
        if (e.detail && e.detail.itemId === item.id) {
          renderBookmarks(item);
        }
      };

      if (currentBookmarkListener) {
        window.removeEventListener('bookmark-added', currentBookmarkListener);
      }
      window.addEventListener('bookmark-added', onBookmarkUpdate);
      currentBookmarkListener = onBookmarkUpdate;
    } else {
      // Clear listener if we navigate to an item without audio
      if (currentBookmarkListener) {
        window.removeEventListener('bookmark-added', currentBookmarkListener);
        currentBookmarkListener = null;
      }
    }

    // RSS Feed Rendering & Logic
    const rssStatusBadge = document.getElementById('rss-status-badge');
    const rssControls = document.getElementById('rss-controls');
    
    async function updateRssSection() {
      if (!rssStatusBadge || !rssControls) return;
      try {
        const feedsResp = await request('GET', '/api/feeds');
        const feeds = feedsResp.feeds || [];
        const activeFeed = feeds.find(f => f.entityId === item.id);
        
        if (activeFeed) {
          rssStatusBadge.className = 'px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-accent/20 text-accent';
          rssStatusBadge.textContent = 'Active';
          
          rssControls.innerHTML = `
            <div class="space-y-1.5">
              <label class="text-[0.65rem] text-black-100 font-semibold uppercase">Feed URL</label>
              <div class="flex gap-1.5">
                <input type="text" id="rss-feed-url-input" readonly value="${escapeHtml(activeFeed.feedUrl)}" class="flex-grow bg-black-500 text-white font-mono text-[0.7rem] px-2.5 py-1.5 rounded border border-black-300 focus:outline-none select-all">
                <button id="rss-copy-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-3 rounded transition-colors text-[0.7rem]">
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
              if (!confirm('Are you sure you want to close this RSS feed?')) return;
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
              <p class="text-black-100 text-[0.7rem]">Generate a public RSS feed to subscribe to this item in external podcast players.</p>
              <button id="rss-action-btn" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-1.5 px-3 rounded transition-all text-[0.7rem]">
                Open Public RSS Feed
              </button>
            `;
            
            const actionBtn = document.getElementById('rss-action-btn');
            if (actionBtn) {
              actionBtn.onclick = async () => {
                try {
                  await request('POST', '/api/feeds', {
                    entityId: item.id,
                    type: mediaType
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

    // 1. Hook playlist button
    const playlistActionBtn = document.getElementById('details-playlist-action-btn');
    if (playlistActionBtn) {
      playlistActionBtn.onclick = () => {
        triggerAddToPlaylistModal(item, libraryId);
      };
    }

    // 2. Hook download button
    const downloadActionBtn = document.getElementById('details-download-action-btn');
    if (downloadActionBtn) {
      downloadActionBtn.onclick = () => {
        const token = localStorage.getItem('token');
        const url = resolvePath(`/api/items/${item.id}/download?token=${token}`);
        window.open(url, '_blank');
      };
    }

    // 3. Hook delete item button (admins only)
    if (isAdmin) {
      const deleteItemBtn = document.getElementById('details-delete-item-btn');
      if (deleteItemBtn) {
        deleteItemBtn.onclick = async () => {
          if (!confirm(`Are you sure you want to delete the library item "${title}"? This will permanently delete the item and its media progress.`)) {
            return;
          }
          try {
            await request('DELETE', `/api/items/${item.id}`);
            alert('Library item deleted successfully.');
            backCallback();
          } catch (err) {
            alert('Failed to delete library item: ' + err.message);
          }
        };
      }
    }

    // 4. Fetch and render progress details
    if (hasAudio || hasEbook) {
      request('GET', `/api/me/progress/${item.id}`)
        .then(progressObj => {
          if (progressObj) {
            const progressSection = document.getElementById('details-progress-section');
            if (progressSection) {
              progressSection.classList.remove('hidden');
              
              // Update badge
              const badge = document.getElementById('progress-status-badge');
              if (progressObj.isFinished) {
                badge.className = 'px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-success text-white';
                badge.textContent = 'Finished';
              } else if (progressObj.progress > 0) {
                badge.className = 'px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-yellow-500 text-white';
                badge.textContent = 'In Progress';
              } else {
                badge.className = 'px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-black-500 text-black-100';
                badge.textContent = 'Not Started';
              }

              // Update progress bar
              const percent = progressObj.progress ? Math.round(progressObj.progress * 100) : 0;
              const fill = document.getElementById('progress-bar-fill');
              if (fill) {
                fill.style.width = `${percent}%`;
                if (progressObj.isFinished) {
                  fill.className = 'h-full bg-success';
                } else {
                  fill.className = 'h-full bg-accent';
                }
              }

              // Update percentage text
              const percentText = document.getElementById('progress-percent');
              if (percentText) percentText.textContent = `${percent}%`;

              // Duration formatting helper
              const duration = progressObj.duration || item.media?.duration || 0;
              const currentTime = progressObj.currentTime || 0;

              const timeListened = document.getElementById('progress-time-listened');
              if (timeListened) timeListened.textContent = formatDuration(currentTime);

              const timeRemaining = document.getElementById('progress-time-remaining');
              if (timeRemaining) {
                const remaining = duration - currentTime;
                timeRemaining.textContent = remaining > 0 ? `${formatDuration(remaining)} left` : '00:00:00 left';
              }

              // Update buttons
              const toggleBtnText = document.getElementById('progress-toggle-finished-text');
              const toggleBtnIcon = document.querySelector('#progress-toggle-finished-btn span');
              if (toggleBtnText && toggleBtnIcon) {
                if (progressObj.isFinished) {
                  toggleBtnText.textContent = 'Mark Unfinished';
                  toggleBtnIcon.textContent = 'history';
                } else {
                  toggleBtnText.textContent = 'Mark Finished';
                  toggleBtnIcon.textContent = 'check_circle';
                }
              }

              // Wire up progress buttons
              const toggleBtn = document.getElementById('progress-toggle-finished-btn');
              if (toggleBtn) {
                toggleBtn.onclick = async () => {
                  try {
                    const isFinished = !progressObj.isFinished;
                    const durationVal = duration;
                    const currentTimeVal = isFinished ? durationVal : 0;
                    
                    await request('PATCH', `/api/me/progress/${item.id}`, {
                      isFinished,
                      currentTime: currentTimeVal,
                      duration: durationVal
                    });
                    
                    // Reload item details to reflect changes
                    loadItemDetails(itemId, libraryId, backCallback);
                  } catch (err) {
                    alert('Failed to update progress: ' + err.message);
                  }
                };
              }

              const resetBtn = document.getElementById('progress-reset-btn');
              if (resetBtn) {
                resetBtn.onclick = async () => {
                  if (!confirm('Are you sure you want to reset your listening progress?')) return;
                  try {
                    await request('DELETE', `/api/me/progress/${item.id}`);
                    loadItemDetails(itemId, libraryId, backCallback);
                  } catch (err) {
                    alert('Failed to reset progress: ' + err.message);
                  }
                };
              }
            }
          } else {
            // No progress object exists yet, but we should still show the "Not Started" state if it has audio or ebook
            const progressSection = document.getElementById('details-progress-section');
            if (progressSection) {
              progressSection.classList.remove('hidden');
              
              const badge = document.getElementById('progress-status-badge');
              if (badge) {
                badge.className = 'px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-black-500 text-black-100';
                badge.textContent = 'Not Started';
              }

              const fill = document.getElementById('progress-bar-fill');
              if (fill) {
                fill.style.width = '0%';
                fill.className = 'h-full bg-accent';
              }

              const percentText = document.getElementById('progress-percent');
              if (percentText) percentText.textContent = '0%';

              const timeListened = document.getElementById('progress-time-listened');
              if (timeListened) timeListened.textContent = '00:00:00';

              const timeRemaining = document.getElementById('progress-time-remaining');
              if (timeRemaining) {
                const duration = item.media?.duration || 0;
                timeRemaining.textContent = duration > 0 ? `${formatDuration(duration)} left` : '00:00:00 left';
              }

              const toggleBtnText = document.getElementById('progress-toggle-finished-text');
              const toggleBtnIcon = document.querySelector('#progress-toggle-finished-btn span');
              if (toggleBtnText && toggleBtnIcon) {
                toggleBtnText.textContent = 'Mark Finished';
                toggleBtnIcon.textContent = 'check_circle';
              }

              const toggleBtn = document.getElementById('progress-toggle-finished-btn');
              if (toggleBtn) {
                toggleBtn.onclick = async () => {
                  try {
                    const durationVal = item.media?.duration || 0;
                    await request('PATCH', `/api/me/progress/${item.id}`, {
                      isFinished: true,
                      currentTime: durationVal,
                      duration: durationVal
                    });
                    loadItemDetails(itemId, libraryId, backCallback);
                  } catch (err) {
                    alert('Failed to update progress: ' + err.message);
                  }
                };
              }

              const resetBtn = document.getElementById('progress-reset-btn');
              if (resetBtn) {
                resetBtn.classList.add('hidden');
              }
            }
          }
        })
        .catch(err => {
          console.warn('Failed to load media progress details:', err);
        });
    }

  } catch (err) {
    console.error('Failed to load item details:', err);
    container.innerHTML = `
      <div class="p-6 text-center text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load item details: ${escapeHtml(err.message)}</p>
        <button id="details-error-back-btn" class="mt-4 bg-black-500 hover:bg-black-400 text-white px-4 py-2 rounded text-xs">Go Back</button>
      </div>
    `;
    const errBackBtn = document.getElementById('details-error-back-btn');
    if (errBackBtn) errBackBtn.onclick = backCallback;
  }
}

/**
 * Triggers a beautiful Modal to add an item to an existing or new playlist.
 */

/**
 * Split helper
 */
export function splitCommaList(str) {
  if (!str) return [];
  return str.split(',')
    .map(val => val.trim())
    .filter(val => val.length > 0);
}

/**
 * Format bytes helper
 */
export function formatBytes(bytes) {
  if (!bytes) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

/**
 * Format duration helper
 */
export function formatDuration(seconds) {
  if (!seconds) return '0:00';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);

  const mStr = m < 10 && h > 0 ? '0' + m : m;
  const sStr = s < 10 ? '0' + s : s;

  if (h > 0) {
    return `${h}:${mStr}:${sStr}`;
  }
  return `${mStr}:${sStr}`;
}

export function escapeHtml(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

export function getDiffOldHtml(oldStr, newStr) {
  if (!oldStr) return '';
  if (!newStr) return `<span class="bg-red-950/80 text-red-400 line-through px-1 rounded font-medium">${escapeHtml(oldStr)}</span>`;
  if (oldStr === newStr) return escapeHtml(oldStr);
  
  const oldWords = oldStr.split(/(\s+)/);
  const newCleanWords = newStr.split(/\s+/).map(w => w.toLowerCase().replace(/[.,\/#!$%\^&\*;:{}=\-_`~()]/g,""));
  const newSet = new Set(newCleanWords);
  
  return oldWords.map(word => {
    if (/\s+/.test(word)) return word;
    const cleanWord = word.toLowerCase().replace(/[.,\/#!$%\^&\*;:{}=\-_`~()]/g,"");
    if (cleanWord === '' || newSet.has(cleanWord)) {
      return escapeHtml(word);
    } else {
      return `<span class="bg-red-950/80 text-red-400 line-through px-0.5 rounded font-medium">${escapeHtml(word)}</span>`;
    }
  }).join('');
}

export function getDiffNewHtml(oldStr, newStr) {
  if (!newStr) return '';
  if (!oldStr) return `<span class="bg-green-950/80 text-green-400 px-1 rounded font-bold">+ ${escapeHtml(newStr)}</span>`;
  if (oldStr === newStr) return escapeHtml(oldStr);
  
  const newWords = newStr.split(/(\s+)/);
  const oldCleanWords = oldStr.split(/\s+/).map(w => w.toLowerCase().replace(/[.,\/#!$%\^&\*;:{}=\-_`~()]/g,""));
  const oldSet = new Set(oldCleanWords);
  
  return newWords.map(word => {
    if (/\s+/.test(word)) return word;
    const cleanWord = word.toLowerCase().replace(/[.,\/#!$%\^&\*;:{}=\-_`~()]/g,"");
    if (cleanWord === '' || oldSet.has(cleanWord)) {
      return escapeHtml(word);
    } else {
      return `<span class="bg-green-950/80 text-green-300 px-0.5 rounded font-bold">${escapeHtml(word)}</span>`;
    }
  }).join('');
}

