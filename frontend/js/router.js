import { request, resolvePath, ROUTER_BASE_PATH } from './api.js';
import { getActiveLibraryId, getActiveLibrary } from './library.js';
import { loadDashboard } from './dashboard.js';
import { loadPlaylists, loadPlaylistDetails } from './playlists.js';
import { loadCollections, loadCollectionDetails } from './collections.js';
import { loadItemDetails } from './itemDetails.js';
import { loadAuthors, loadSeries, loadAuthorDetails, loadSeriesDetails } from './authors.js';
import { loadNarrators } from './narrators.js';
import { loadStats } from './stats.js';
import { loadPodcastLatestView, loadPodcastAddView, loadPodcastDownloadQueueView } from './podcasts.js';
import { loadSettings } from './settings.js';

export function highlightSidebarLink(pageName) {
  const sidebarLinks = document.querySelectorAll('#siderail-buttons-container a');
  sidebarLinks.forEach(link => {
    const p = link.querySelector('p');
    if (!p) return;
    const name = p.textContent.trim();
    if (name === pageName) {
      link.classList.add('active');
      p.classList.add('font-semibold');
      const activeBar = link.querySelector('.active-indicator');
      if (activeBar) {
        activeBar.classList.add('active');
      }
    } else {
      link.classList.remove('active');
      p.classList.remove('font-semibold');
      const activeBar = link.querySelector('.active-indicator');
      if (activeBar) {
        activeBar.classList.remove('active');
      }
    }
  });
}

export async function updateSidebarVisibility() {
  const lib = getActiveLibrary();
  const user = window.currentUser || {};
  const isAdmin = user.type === 'admin' || user.type === 'root';

  // Toggle book specific links
  const bookLinks = [
    document.getElementById('sidebar-series'),
    document.getElementById('sidebar-collections'),
    document.getElementById('sidebar-playlists'),
    document.getElementById('sidebar-authors'),
    document.getElementById('sidebar-narrators')
  ];
  
  // Toggle podcast specific links
  const podcastLinks = [
    document.getElementById('sidebar-latest')
  ];

  const sidebarStats = document.getElementById('sidebar-stats');
  const sidebarAdd = document.getElementById('sidebar-add');
  const sidebarDownloadQueue = document.getElementById('sidebar-download-queue');
  const sidebarIssues = document.getElementById('sidebar-issues');

  const isBook = lib && (lib.mediaType === 'book' || lib.icon === 'audiobooks');
  const isPodcast = lib && (lib.mediaType === 'podcast' || lib.icon === 'podcasts');

  bookLinks.forEach(link => {
    if (link) {
      if (isBook) {
        link.classList.remove('hidden');
      } else {
        link.classList.add('hidden');
      }
    }
  });

  podcastLinks.forEach(link => {
    if (link) {
      if (isPodcast) {
        link.classList.remove('hidden');
      } else {
        link.classList.add('hidden');
      }
    }
  });

  if (sidebarStats) {
    if (isBook || isPodcast) {
      sidebarStats.classList.remove('hidden');
    } else {
      sidebarStats.classList.add('hidden');
    }
  }

  if (sidebarAdd) {
    if (isPodcast && isAdmin) {
      sidebarAdd.classList.remove('hidden');
    } else {
      sidebarAdd.classList.add('hidden');
    }
  }

  if (sidebarDownloadQueue) {
    if (isPodcast && isAdmin) {
      sidebarDownloadQueue.classList.remove('hidden');
    } else {
      sidebarDownloadQueue.classList.add('hidden');
    }
  }

  // Update issues count and badge
  if (lib) {
    try {
      const data = await request('GET', `/api/libraries/${lib.id}/filterdata`);
      if (data && data.numIssues > 0) {
        if (sidebarIssues) {
          sidebarIssues.classList.remove('hidden');
          const badgeText = sidebarIssues.querySelector('.issues-badge-text');
          if (badgeText) badgeText.textContent = data.numIssues;
        }
      } else {
        if (sidebarIssues) sidebarIssues.classList.add('hidden');
      }
    } catch (err) {
      console.warn('Failed to load filterdata for issues:', err);
      if (sidebarIssues) sidebarIssues.classList.add('hidden');
    }
  } else {
    if (sidebarIssues) sidebarIssues.classList.add('hidden');
  }
}
window.updateSidebarVisibility = updateSidebarVisibility;

export function isDashboardActive() {
  const activeLink = document.querySelector('#siderail-buttons-container a.bg-primary\\/80');
  if (!activeLink) return false;
  const pageName = activeLink.querySelector('p').textContent.trim();
  const hasDetailsBtn = !!document.getElementById('details-back-btn');
  return (pageName === 'Home' || pageName === 'Library') && !hasDetailsBtn;
}

export function showNoLibrariesWelcome() {
  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Welcome';

  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = '0 Books';

  const bookshelfContainer = document.getElementById('bookshelf');
  if (bookshelfContainer) {
    bookshelfContainer.innerHTML = `
      <div class="flex flex-col items-center justify-center text-center p-8 mt-12 space-y-6 max-w-lg mx-auto bg-primary border border-black-300 rounded-lg shadow-xl">
        <img src="assets/images/icon.svg" alt="Audiobookshelf" class="w-20 h-20 mb-2">
        <h2 class="text-2xl font-bold text-white">Welcome to Audiobookshelf!</h2>
        <p class="text-gray-300 text-sm leading-relaxed">
          Create a library to store your audiobooks and podcasts. You can configure directories to scan for files and customize your listening experience.
        </p>
        <button type="button" id="btn-welcome-add-library" class="bg-accent hover:opacity-90 text-primary font-bold px-6 py-2.5 rounded-md transition duration-150 flex items-center space-x-2 text-sm shadow-md cursor-pointer">
          <span class="material-symbols text-lg">add</span>
          <span>Add Your First Library</span>
        </button>
      </div>
    `;
    
    const welcomeAddBtn = document.getElementById('btn-welcome-add-library');
    if (welcomeAddBtn) {
      welcomeAddBtn.onclick = () => {
        window.location.hash = 'libraries';
        navigateTo('/settings');
        // Wait a tiny bit and click the Add Library button in settings
        setTimeout(() => {
          const createBtn = document.getElementById('btn-create-library');
          if (createBtn) {
            createBtn.click();
          }
        }, 300);
      };
    }
  }
}

export function navigateTo(path, pushState = true) {
  if (window.cleanupSettings) {
    try {
      window.cleanupSettings();
    } catch (e) {
      console.error(e);
    }
    window.cleanupSettings = null;
  }
  const resolved = resolvePath(path);
  if (pushState) {
    window.history.pushState(null, '', resolved);
  }

  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  let relPath = window.location.pathname;
  if (ROUTER_BASE_PATH && relPath.startsWith(ROUTER_BASE_PATH)) {
    relPath = relPath.substring(ROUTER_BASE_PATH.length);
  }
  if (!relPath.startsWith('/')) {
    relPath = '/' + relPath;
  }

  const performTransition = () => {
    const filterBtn = document.getElementById('filter-dropdown-btn');
    const sortBtn = document.getElementById('sort-dropdown-btn');
    const sortOrderToggle = document.getElementById('sort-order-toggle-btn');
    const shelfSizeControl = document.getElementById('shelf-size-control');
    const styleSwitcher = document.getElementById('style-switcher');

    const showControls = (relPath === '/library');
    const showBookCount = (relPath === '/library' || relPath === '/series' || relPath === '/authors' || relPath === '/collections' || relPath === '/playlists' || relPath === '/narrators');
    const showShelfSize = (
      relPath === '/' || 
      relPath === '/library' || 
      relPath === '/series' || 
      relPath === '/authors' || 
      relPath === '/collections' || 
      relPath === '/playlists' || 
      relPath === '/narrators' ||
      relPath.startsWith('/author/') ||
      relPath.startsWith('/series/') ||
      relPath.startsWith('/playlist/') ||
      relPath.startsWith('/collection/')
    );
    
    const bookCount = document.getElementById('book-count');
    const viewTitleSeparator = document.getElementById('view-title-separator');
    if (bookCount) {
      if (showBookCount) bookCount.classList.remove('hidden');
      else bookCount.classList.add('hidden');
    }
    if (viewTitleSeparator) {
      if (showBookCount) viewTitleSeparator.classList.remove('hidden');
      else viewTitleSeparator.classList.add('hidden');
    }

    const globalSearchInput = document.getElementById('global-search-input');
    const globalSearchClearBtn = document.getElementById('global-search-clear-btn');
    const globalSearchDropdown = document.getElementById('global-search-dropdown');
    if (!showControls) {
      if (globalSearchInput) globalSearchInput.value = '';
      if (globalSearchClearBtn) globalSearchClearBtn.classList.add('hidden');
      if (globalSearchDropdown) globalSearchDropdown.classList.add('hidden');
    }
    
    if (filterBtn) {
      if (showControls) filterBtn.parentElement.classList.remove('hidden');
      else filterBtn.parentElement.classList.add('hidden');
    }
    if (sortBtn) {
      if (showControls) sortBtn.parentElement.classList.remove('hidden');
      else sortBtn.parentElement.classList.add('hidden');
    }
    if (sortOrderToggle) {
      if (showControls) sortOrderToggle.classList.remove('hidden');
      else sortOrderToggle.classList.add('hidden');
    }
    if (shelfSizeControl) {
      const currentStyle = localStorage.getItem('library-style') || 'shelf';
      if (showShelfSize && (relPath !== '/library' || currentStyle !== 'list')) shelfSizeControl.classList.remove('hidden');
      else shelfSizeControl.classList.add('hidden');
    }
    if (styleSwitcher) {
      if (showControls) styleSwitcher.classList.remove('hidden');
      else styleSwitcher.classList.add('hidden');
    }

    const savePresetBtn = document.getElementById('save-preset-btn');
    const presetsContainer = document.getElementById('presets-pills-container');
    if (savePresetBtn) {
      if (showControls) savePresetBtn.classList.remove('hidden');
      else savePresetBtn.classList.add('hidden');
    }
    if (presetsContainer) {
      if (showControls) presetsContainer.classList.remove('hidden');
      else presetsContainer.classList.add('hidden');
    }

    const activeLibId = getActiveLibraryId();

    if (relPath === '/') {
      const isMissing = localStorage.getItem('library-filterBy') === 'missing';
      if (isMissing) {
        highlightSidebarLink('Issues');
      } else {
        highlightSidebarLink('Home');
      }
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) viewTitle.textContent = isMissing ? 'Issues' : 'Home';
      if (activeLibId) {
        loadDashboard(activeLibId, true);
      } else {
        showNoLibrariesWelcome();
      }
    } else if (relPath === '/library') {
      highlightSidebarLink('Library');
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) {
        const lib = getActiveLibrary();
        viewTitle.textContent = lib ? lib.name : 'Library';
      }
      if (activeLibId) {
        loadDashboard(activeLibId, false);
      } else {
        showNoLibrariesWelcome();
      }
    } else if (relPath === '/playlists') {
      highlightSidebarLink('Playlists');
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) viewTitle.textContent = 'Playlists';
      if (activeLibId) loadPlaylists(activeLibId);
    } else if (relPath === '/collections') {
      highlightSidebarLink('Collections');
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) viewTitle.textContent = 'Collections';
      if (activeLibId) loadCollections(activeLibId);
    } else if (relPath === '/authors') {
      highlightSidebarLink('Authors');
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) viewTitle.textContent = 'Authors';
      if (activeLibId) loadAuthors(activeLibId);
    } else if (relPath === '/series') {
      highlightSidebarLink('Series');
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) viewTitle.textContent = 'Series';
      if (activeLibId) loadSeries(activeLibId);
    } else if (relPath === '/narrators') {
      highlightSidebarLink('Narrators');
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) viewTitle.textContent = 'Narrators';
      if (activeLibId) loadNarrators(activeLibId);
    } else if (relPath === '/stats') {
      highlightSidebarLink('Stats');
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) viewTitle.textContent = 'Stats';
      loadStats();
    } else if (relPath === '/podcast/latest') {
      highlightSidebarLink('Latest');
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) viewTitle.textContent = 'Latest Episodes';
      if (activeLibId) loadPodcastLatestView(activeLibId);
    } else if (relPath === '/podcast/add') {
      highlightSidebarLink('Add');
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) viewTitle.textContent = 'Add Podcast';
      if (activeLibId) loadPodcastAddView(activeLibId);
    } else if (relPath === '/podcast/download-queue') {
      highlightSidebarLink('Download Queue');
      const viewTitle = document.getElementById('view-title');
      if (viewTitle) viewTitle.textContent = 'Download Queue';
      if (activeLibId) loadPodcastDownloadQueueView(activeLibId);
    } else if (relPath === '/settings') {
      // Deselect sidebar highlights
      document.querySelectorAll('#siderail-buttons-container a').forEach(l => {
        l.classList.remove('active');
        const activeBar = l.querySelector('.active-indicator');
        if (activeBar) activeBar.classList.remove('active');
      });
      loadSettings();
    } else if (relPath.startsWith('/item/')) {
      const itemId = relPath.substring('/item/'.length);
      if (itemId) {
        loadItemDetails(itemId, activeLibId, () => {
          if (window.history.length > 1) {
            window.history.back();
          } else {
            navigateTo('/');
          }
        });
      }
    } else if (relPath.startsWith('/author/')) {
      const authorId = relPath.substring('/author/'.length);
      if (authorId) {
        highlightSidebarLink('Authors');
        loadAuthorDetails(authorId);
      }
    } else if (relPath.startsWith('/series/')) {
      const seriesId = relPath.substring('/series/'.length);
      if (seriesId) {
        highlightSidebarLink('Series');
        loadSeriesDetails(seriesId);
      }
    } else if (relPath.startsWith('/playlist/')) {
      const playlistId = relPath.substring('/playlist/'.length);
      if (playlistId) {
        highlightSidebarLink('Playlists');
        const viewTitle = document.getElementById('view-title');
        if (viewTitle) viewTitle.textContent = 'Playlist Details';
        if (activeLibId) loadPlaylistDetails(playlistId, activeLibId);
      }
    } else if (relPath.startsWith('/collection/')) {
      const collectionId = relPath.substring('/collection/'.length);
      if (collectionId) {
        highlightSidebarLink('Collections');
        const viewTitle = document.getElementById('view-title');
        if (viewTitle) viewTitle.textContent = 'Collection Details';
        if (activeLibId) loadCollectionDetails(collectionId, activeLibId);
      }
    } else {
      highlightSidebarLink('Home');
      if (activeLibId) loadDashboard(activeLibId, true);
    }
  };

  if (document.startViewTransition) {
    document.startViewTransition(() => {
      performTransition();
    });
  } else {
    performTransition();
  }
}
window.navigateTo = navigateTo;

window.addEventListener('popstate', () => {
  let relPath = window.location.pathname;
  const basePath = window.ROUTER_BASE_PATH || '';
  if (basePath && relPath.startsWith(basePath)) {
    relPath = relPath.substring(basePath.length);
  }
  if (!relPath.startsWith('/')) {
    relPath = '/' + relPath;
  }

  const isCurrentlySettings = !!document.getElementById('settings-tabs');
  if (isCurrentlySettings && relPath === '/settings') {
    return;
  }
  navigateTo(window.location.pathname, false);
});
