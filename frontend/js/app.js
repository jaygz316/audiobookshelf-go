// js/app.js

import { initAuth, showAppContainer, logout } from './auth.js';
import { initLibrary, getActiveLibraryId, getActiveLibrary } from './library.js';
import { loadDashboard } from './dashboard.js';
import { request, resolvePath, ROUTER_BASE_PATH } from './api.js';
import { connectSocket, disconnectSocket, onEvent } from './socket.js';
import { loadSettings, applyServerThemeAndCss } from './settings.js';
import { loadPlaylists, loadPlaylistDetails } from './playlists.js';
import { loadCollections, loadCollectionDetails } from './collections.js';
import { loadItemDetails } from './itemDetails.js';
import { loadAuthors, loadSeries, loadAuthorDetails, loadSeriesDetails } from './authors.js';
import { loadNarrators } from './narrators.js';
import { loadStats } from './stats.js';
import { initPublicShare } from './publicShare.js';
import { initSearchPresets } from './presets.js';
import { loadPodcastLatestView, loadPodcastAddView, loadPodcastDownloadQueueView } from './podcasts.js';

function initApp() {
  setupEventHandlers();
  
  // Check if this is a public share path (/s/slug)
  const path = window.location.pathname;
  const segments = path.split('/');
  const sIdx = segments.indexOf('s');
  if (sIdx !== -1 && sIdx < segments.length - 1) {
    const slug = segments[sIdx + 1];
    if (slug) {
      initPublicShare(slug);
      return;
    }
  }

  // Initialize Auth on page load
  initAuth().then(payload => {
    if (payload) {
      bootstrapApp(payload);
    }
  });
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initApp);
} else {
  initApp();
}

function highlightSidebarLink(pageName) {
  const sidebarLinks = document.querySelectorAll('#siderail-buttons-container a');
  sidebarLinks.forEach(link => {
    const p = link.querySelector('p');
    if (!p) return;
    const name = p.textContent.trim();
    if (name === pageName) {
      link.classList.remove('hover:bg-primary', 'text-white/80', 'bg-bg/60');
      link.classList.add('bg-primary/80', 'text-white');
      const activeBar = link.querySelector('.active-indicator');
      if (activeBar) activeBar.classList.remove('hidden');
    } else {
      link.classList.remove('bg-primary/80', 'text-white');
      link.classList.add('hover:bg-primary', 'text-white/80', 'bg-bg/60');
      const activeBar = link.querySelector('.active-indicator');
      if (activeBar) activeBar.classList.add('hidden');
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
    if (isBook && isAdmin) {
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

function setupEventHandlers() {
  // Credentials Form Submission
  const loginForm = document.getElementById('login-form');
  if (loginForm) {
    loginForm.onsubmit = async (e) => {
      e.preventDefault();
      const usernameInput = document.getElementById('username');
      const passwordInput = document.getElementById('password');
      const loginError = document.getElementById('login-error');
      const submitBtn = document.getElementById('login-submit-btn');

      loginError.classList.add('hidden');
      loginError.textContent = '';

      if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.textContent = 'Checking...';
      }

      try {
        const credentials = {
          username: usernameInput.value,
          password: passwordInput.value
        };
        const response = await request('POST', '/login', credentials);
        const token = response.user?.accessToken || response.user?.token;
        if (token) {
          localStorage.setItem('token', token);
          bootstrapApp(response);
        } else {
          throw new Error('Invalid response structure from server');
        }
      } catch (err) {
        console.error('Login failed:', err);
        loginError.textContent = err.message || 'Invalid username or password';
        loginError.classList.remove('hidden');
      } finally {
        if (submitBtn) {
          submitBtn.disabled = false;
          submitBtn.textContent = 'Submit';
        }
      }
    };
  }

  // Logout Action
  const logoutBtn = document.getElementById('logout-btn');
  if (logoutBtn) {
    logoutBtn.onclick = (e) => {
      e.preventDefault();
      disconnectSocket();
      logout();
    };
  }

  // Global dropdown closer
  window.closeAllDropdowns = (exceptMenu = null) => {
    const ids = ['library-dropdown-menu', 'user-dropdown', 'filter-dropdown-menu', 'sort-dropdown-menu'];
    ids.forEach(id => {
      const m = document.getElementById(id);
      if (m && m !== exceptMenu) {
        if (typeof m.closeDropdown === 'function') {
          m.closeDropdown();
        } else {
          m.classList.add('hidden');
        }
      }
    });
  };

  // Global modal transition animator
  window.animateModal = (modalEl, contentQuery = '.bg-primary') => {
    // 1. Prepare initial classes
    modalEl.classList.add('transition-opacity', 'duration-200', 'ease-out', 'opacity-0');
    
    // Find dialog container (usually has class '.bg-primary' or is the direct first child)
    const contentEl = modalEl.querySelector(contentQuery) || modalEl.firstElementChild;
    if (contentEl) {
      contentEl.classList.add('transition-all', 'duration-200', 'ease-out', 'transform', 'scale-95', 'opacity-0');
    }
    
    // Override remove method to hook closing transition
    const originalRemove = modalEl.remove.bind(modalEl);
    let isClosing = false;
    
    modalEl.closeModal = () => {
      if (isClosing) return;
      isClosing = true;
      
      modalEl.classList.remove('opacity-100');
      modalEl.classList.add('opacity-0');
      
      if (contentEl) {
        contentEl.classList.remove('scale-100', 'opacity-100');
        contentEl.classList.add('scale-95', 'opacity-0');
      }
      
      const onTransitionEnd = (e) => {
        if (e.target === modalEl || e.target === contentEl) {
          modalEl.removeEventListener('transitionend', onTransitionEnd);
          originalRemove();
        }
      };
      modalEl.addEventListener('transitionend', onTransitionEnd);
      
      // Safety timeout: force remove after 250ms if transition event is missed
      setTimeout(() => {
        try { originalRemove(); } catch (err) {}
      }, 250);
    };
    
    modalEl.remove = modalEl.closeModal;
    
    // Trigger entry transition in next tick
    setTimeout(() => {
      modalEl.classList.remove('opacity-0');
      modalEl.classList.add('opacity-100');
      if (contentEl) {
        contentEl.classList.remove('scale-95', 'opacity-0');
        contentEl.classList.add('scale-100', 'opacity-100');
      }
    }, 20);
  };

  // Intercept document.body.appendChild to automatically animate newly created modals
  const originalAppendChild = document.body.appendChild.bind(document.body);
  document.body.appendChild = function(node) {
    if (node && node.nodeType === Node.ELEMENT_NODE && node.classList.contains('fixed') && node.classList.contains('inset-0')) {
      window.animateModal(node);
    }
    return originalAppendChild(node);
  };


  // User Dropdown Toggles
  const userMenuBtn = document.getElementById('user-menu-btn');
  const userDropdown = document.getElementById('user-dropdown');
  if (userMenuBtn && userDropdown) {
    let isOpen = false;
    userDropdown.classList.add('transition-all', 'duration-150', 'ease-out', 'transform', 'scale-95', 'opacity-0');
    
    const closeUserDropdown = () => {
      if (!isOpen) return;
      isOpen = false;
      userDropdown.classList.remove('scale-100', 'opacity-100');
      userDropdown.classList.add('scale-95', 'opacity-0');
      const handleTransitionEnd = (e) => {
        if (e.propertyName === 'opacity' && !isOpen) {
          userDropdown.classList.add('hidden');
          userDropdown.removeEventListener('transitionend', handleTransitionEnd);
        }
      };
      userDropdown.addEventListener('transitionend', handleTransitionEnd);
    };

    userMenuBtn.onclick = (e) => {
      e.stopPropagation();
      window.closeAllDropdowns(userDropdown);
      if (isOpen) {
        closeUserDropdown();
      } else {
        isOpen = true;
        userDropdown.classList.remove('hidden');
        userDropdown.offsetHeight; // reflow
        userDropdown.classList.remove('scale-95', 'opacity-0');
        userDropdown.classList.add('scale-100', 'opacity-100');
      }
    };

    userDropdown.closeDropdown = closeUserDropdown;

    document.addEventListener('click', () => {
      closeUserDropdown();
    });
    userDropdown.onclick = (e) => {
      e.stopPropagation();
    };
  }

  // Initialize Title Separator MutationObserver
  const bookCount = document.getElementById('book-count');
  const separator = document.getElementById('view-title-separator');
  if (bookCount && separator) {
    const observer = new MutationObserver(() => {
      if (bookCount.textContent.trim()) {
        separator.classList.remove('hidden');
      } else {
        separator.classList.add('hidden');
      }
    });
    observer.observe(bookCount, { childList: true, characterData: true, subtree: true });
    // Initial check
    if (bookCount.textContent.trim()) {
      separator.classList.remove('hidden');
    } else {
      separator.classList.add('hidden');
    }
  }

  // Siderail buttons static toggles (for Milestones)
  const sidebarLinks = document.querySelectorAll('#siderail-buttons-container a');
  sidebarLinks.forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      
      const pEl = link.querySelector('p');
      if (!pEl) return;
      const pageName = pEl.textContent.trim();
      let path = '/';
      if (pageName === 'Playlists') path = '/playlists';
      else if (pageName === 'Collections') path = '/collections';
      else if (pageName === 'Authors') path = '/authors';
      else if (pageName === 'Series') path = '/series';
      else if (pageName === 'Narrators') path = '/narrators';
      else if (pageName === 'Stats') path = '/stats';
      else if (pageName === 'Latest') path = '/podcast/latest';
      else if (pageName === 'Add') path = '/podcast/add';
      else if (pageName === 'Download Queue') path = '/podcast/download-queue';
      else if (pageName === 'Issues') {
        localStorage.setItem('library-filterBy', 'missing');
        window.dispatchEvent(new CustomEvent('navigate-to-dashboard', {
          detail: {
            filterBy: 'missing',
            filterLabel: 'Issues'
          }
        }));
        return;
      }

      navigateTo(path);
    });
  });

  // Global Listeners for Modular Events
  window.addEventListener('auth-unauthorized', () => {
    disconnectSocket();
    logout();
  });

  window.addEventListener('library-changed', (e) => {
    const libraryId = e.detail.libraryId;
    if (!libraryId) return;
    
    updateSidebarVisibility();
    
    // Reload the current active page content without changing URL path
    let relPath = window.location.pathname;
    if (ROUTER_BASE_PATH && relPath.startsWith(ROUTER_BASE_PATH)) {
      relPath = relPath.substring(ROUTER_BASE_PATH.length);
    }
    if (!relPath.startsWith('/')) {
      relPath = '/' + relPath;
    }

    if (relPath === '/playlists') {
      loadPlaylists(libraryId);
    } else if (relPath === '/collections') {
      loadCollections(libraryId);
    } else if (relPath === '/authors') {
      loadAuthors(libraryId);
    } else if (relPath === '/series') {
      loadSeries(libraryId);
    } else if (relPath === '/narrators') {
      loadNarrators(libraryId);
    } else if (relPath === '/podcast/latest') {
      loadPodcastLatestView(libraryId);
    } else if (relPath === '/podcast/add') {
      loadPodcastAddView(libraryId);
    } else if (relPath === '/podcast/download-queue') {
      loadPodcastDownloadQueueView(libraryId);
    } else if (relPath.startsWith('/playlist/')) {
      const playlistId = relPath.substring('/playlist/'.length);
      loadPlaylistDetails(playlistId, libraryId);
    } else if (relPath.startsWith('/collection/')) {
      const collectionId = relPath.substring('/collection/'.length);
      loadCollectionDetails(collectionId, libraryId);
    } else if (relPath.startsWith('/item/')) {
      const itemId = relPath.substring('/item/'.length);
      loadItemDetails(itemId, libraryId, () => {
        if (window.history.length > 1) {
          window.history.back();
        } else {
          navigateTo('/');
        }
      });
    } else if (relPath.startsWith('/author/')) {
      const authorId = relPath.substring('/author/'.length);
      loadAuthorDetails(authorId);
    } else if (relPath.startsWith('/series/')) {
      const seriesId = relPath.substring('/series/'.length);
      loadSeriesDetails(seriesId);
    } else {
      loadDashboard(libraryId);
    }
  });

  window.addEventListener('navigate-to-dashboard', (e) => {
    const activeLibId = getActiveLibraryId();
    if (!activeLibId) return;
    const { filterBy, filterLabel } = e.detail;

    window.history.pushState(null, '', resolvePath('/'));

    if (filterLabel === 'Issues') {
      highlightSidebarLink('Issues');
    } else {
      highlightSidebarLink('Home');
    }

    const viewTitle = document.getElementById('view-title');
    if (viewTitle) {
      viewTitle.textContent = filterLabel || 'Home';
    }

    loadDashboard(activeLibId, filterBy, filterLabel);
  });

  window.addEventListener('navigate-to-author', (e) => {
    const { authorId } = e.detail;
    navigateTo(`/author/${authorId}`);
  });

  window.addEventListener('navigate-to-series', (e) => {
    const { seriesId } = e.detail;
    navigateTo(`/series/${seriesId}`);
  });

  // User Menu settings/admin clicks
  const settingsBtn = document.getElementById('user-menu-settings-btn');
  const adminBtn = document.getElementById('user-menu-admin-btn');
  const handleSettingsClick = (e) => {
    e.preventDefault();
    navigateTo('/settings');
  };
  if (settingsBtn) settingsBtn.onclick = handleSettingsClick;
  if (adminBtn) adminBtn.onclick = handleSettingsClick;

  // Header Logo & Title navigation clicks
  const logoLink = document.getElementById('header-logo-link');
  const titleLink = document.getElementById('header-title-link');
  if (logoLink) {
    logoLink.onclick = (e) => {
      e.preventDefault();
      navigateTo('/');
    };
  }
  if (titleLink) {
    titleLink.onclick = (e) => {
      e.preventDefault();
      navigateTo('/');
    };
  }

  window.addEventListener('popstate', () => {
    navigateTo(window.location.pathname, false);
  });

  // Shelf Sizing and Sorting/Filtering controls initialization
  const initShelfSizing = () => {
    const decBtn = document.getElementById('shelf-size-dec');
    const incBtn = document.getElementById('shelf-size-inc');
    const valSpan = document.getElementById('shelf-size-val');
    if (!decBtn || !incBtn || !valSpan) return;

    let currentSize = parseInt(localStorage.getItem('bookshelf-card-width')) || 120;
    currentSize = Math.max(80, Math.min(240, currentSize));

    const updateSize = (newSize) => {
      currentSize = Math.max(80, Math.min(240, newSize));
      localStorage.setItem('bookshelf-card-width', currentSize);
      valSpan.textContent = currentSize;
      document.documentElement.style.setProperty('--bookshelf-card-width', `${currentSize}px`);
    };

    updateSize(currentSize);

    decBtn.onclick = (e) => {
      e.stopPropagation();
      updateSize(currentSize - 10);
    };

    incBtn.onclick = (e) => {
      e.stopPropagation();
      updateSize(currentSize + 10);
    };
  };

  initShelfSizing();

  // Sorting/Filtering dropdown handlers (Custom animated dropdowns)
  const initCustomFilterAndSort = () => {
    const filterBtn = document.getElementById('filter-dropdown-btn');
    const filterMenu = document.getElementById('filter-dropdown-menu');
    const sortBtn = document.getElementById('sort-dropdown-btn');
    const sortMenu = document.getElementById('sort-dropdown-menu');

    if (!filterBtn || !filterMenu || !sortBtn || !sortMenu) return;

    // Toggles for Filter
    let filterOpen = false;
    filterMenu.classList.add('transition-all', 'duration-150', 'ease-out', 'transform', 'scale-95', 'opacity-0');
    
    const closeFilter = () => {
      if (!filterOpen) return;
      filterOpen = false;
      filterMenu.classList.remove('scale-100', 'opacity-100');
      filterMenu.classList.add('scale-95', 'opacity-0');
      const handleTransitionEnd = (e) => {
        if (e.propertyName === 'opacity' && !filterOpen) {
          filterMenu.classList.add('hidden');
          filterMenu.removeEventListener('transitionend', handleTransitionEnd);
        }
      };
      filterMenu.addEventListener('transitionend', handleTransitionEnd);
      closeSubmenu();
    };

    filterBtn.onclick = (e) => {
      e.stopPropagation();
      window.closeAllDropdowns(filterMenu);
      if (filterOpen) {
        closeFilter();
      } else {
        filterOpen = true;
        filterMenu.classList.remove('hidden');
        filterMenu.offsetHeight; // reflow
        filterMenu.classList.remove('scale-95', 'opacity-0');
        filterMenu.classList.add('scale-100', 'opacity-100');
        renderFilterMenu();
      }
    };
    filterMenu.closeDropdown = closeFilter;

    // Toggles for Sort
    let sortOpen = false;
    sortMenu.classList.add('transition-all', 'duration-150', 'ease-out', 'transform', 'scale-95', 'opacity-0');
    
    const closeSort = () => {
      if (!sortOpen) return;
      sortOpen = false;
      sortMenu.classList.remove('scale-100', 'opacity-100');
      sortMenu.classList.add('scale-95', 'opacity-0');
      const handleTransitionEnd = (e) => {
        if (e.propertyName === 'opacity' && !sortOpen) {
          sortMenu.classList.add('hidden');
          sortMenu.removeEventListener('transitionend', handleTransitionEnd);
        }
      };
      sortMenu.addEventListener('transitionend', handleTransitionEnd);
    };

    sortBtn.onclick = (e) => {
      e.stopPropagation();
      window.closeAllDropdowns(sortMenu);
      if (sortOpen) {
        closeSort();
      } else {
        sortOpen = true;
        sortMenu.classList.remove('hidden');
        sortMenu.offsetHeight; // reflow
        sortMenu.classList.remove('scale-95', 'opacity-0');
        sortMenu.classList.add('scale-100', 'opacity-100');
      }
    };
    sortMenu.closeDropdown = closeSort;

    // Handle clicks outside
    document.addEventListener('click', () => {
      closeFilter();
      closeSort();
    });

    // Populate and wire options
    const activeFilter = localStorage.getItem('library-filterBy') || '';
    const activeSort = localStorage.getItem('library-sortBy') || 'media.metadata.title';

    let cachedFilterData = null;
    const submenu = document.getElementById('filter-submenu');
    const submenuItems = document.getElementById('filter-submenu-items');
    const searchContainer = document.getElementById('filter-search-container');
    const searchInput = document.getElementById('filter-search-input');

    let currentSubmenuCat = null;
    let submenuItemsData = [];

    const closeSubmenu = () => {
      if (submenu) submenu.classList.add('hidden');
      currentSubmenuCat = null;
    };

    const getDecodedSubVal = (s) => {
      try {
        return decodeURIComponent(escape(atob(s)));
      } catch (e) {
        try {
          return decodeURIComponent(s);
        } catch (err) {
          return s;
        }
      }
    };

    const getFriendlyFilterLabel = (val, filterData) => {
      if (!val) return 'Filter: All';
      const parts = val.split('.');
      const category = parts[0];
      const subVal = parts[1];

      switch (category) {
        case 'progress':
          if (subVal === 'not-started') return 'Unstarted';
          if (subVal === 'in-progress') return 'In Progress';
          if (subVal === 'finished') return 'Completed';
          break;
        case 'authors':
          if (filterData && filterData.authors) {
            const auth = filterData.authors.find(a => a.id === subVal);
            if (auth) return `Author: ${auth.name}`;
          }
          return 'Author';
        case 'series':
          if (subVal === 'no-series') return 'No Series';
          if (filterData && filterData.series) {
            const ser = filterData.series.find(s => s.id === subVal);
            if (ser) return `Series: ${ser.name}`;
          }
          return 'Series';
        case 'narrators':
          return `Narrator: ${getDecodedSubVal(subVal)}`;
        case 'genres':
          return `Genre: ${getDecodedSubVal(subVal)}`;
        case 'tags':
          return `Tag: ${getDecodedSubVal(subVal)}`;
        case 'publishers':
          return `Publisher: ${getDecodedSubVal(subVal)}`;
        case 'languages':
          return `Language: ${getDecodedSubVal(subVal)}`;
        case 'decades':
          return `Decade: ${getDecodedSubVal(subVal)}s`;
        case 'duration':
          if (subVal === 'under-1h') return 'Duration: < 1h';
          if (subVal === '1h-5h') return 'Duration: 1-5h';
          if (subVal === '5h-10h') return 'Duration: 5-10h';
          if (subVal === 'over-10h') return 'Duration: > 10h';
          break;
        case 'missing':
          return 'Missing / Invalid';
      }
      return 'Filtered';
    };

    const updateFilterLabel = (val, filterData) => {
      const labelEl = document.getElementById('filter-selected-label');
      if (labelEl) labelEl.textContent = getFriendlyFilterLabel(val, filterData);
    };

    const renderSubmenuItems = (filterText = '') => {
      if (!submenuItems) return;
      const activeFilterVal = localStorage.getItem('library-filterBy') || '';
      const filtered = submenuItemsData.filter(item => 
        item.label.toLowerCase().includes(filterText.toLowerCase())
      );

      if (filtered.length === 0) {
        submenuItems.innerHTML = `
          <div class="px-3 py-2 text-xs text-black-200">No items found</div>
        `;
        return;
      }

      submenuItems.innerHTML = filtered.map(item => {
        const isSelected = activeFilterVal === item.value;
        return `
          <button class="filter-submenu-option-btn w-full text-left px-3 py-1.5 text-xs text-black-50 hover:bg-black-500 hover:text-white flex items-center justify-between transition-colors focus:outline-none ${isSelected ? 'text-accent font-medium' : ''}" data-value="${item.value}">
            <span class="truncate pr-2">${item.label}</span>
            <span class="material-symbols text-[14px] check-icon ${isSelected ? '' : 'hidden'}">check</span>
          </button>
        `;
      }).join('');

      submenuItems.querySelectorAll('.filter-submenu-option-btn').forEach(btn => {
        btn.onclick = (e) => {
          e.stopPropagation();
          const val = btn.getAttribute('data-value');
          localStorage.setItem('library-filterBy', val);
          updateFilterLabel(val, cachedFilterData);
          closeFilter();
          closeSubmenu();
          const activeLibId = getActiveLibraryId();
          if (activeLibId) loadDashboard(activeLibId);
        };
      });
    };

    const openSubmenu = (cat, data, btnEl) => {
      if (!submenu || !submenuItems) return;
      if (currentSubmenuCat === cat) return;
      currentSubmenuCat = cat;

      submenuItemsData = [];

      switch (cat) {
        case 'progress':
          submenuItemsData = [
            { label: 'Unstarted', value: 'progress.not-started' },
            { label: 'In Progress', value: 'progress.in-progress' },
            { label: 'Completed', value: 'progress.finished' }
          ];
          break;
        case 'authors':
          submenuItemsData = (data.authors || []).map(a => ({
            label: a.name,
            value: `authors.${a.id}`
          }));
          break;
        case 'series':
          submenuItemsData = [
            { label: 'No Series', value: 'series.no-series' },
            ...(data.series || []).map(s => ({
              label: s.name,
              value: `series.${s.id}`
            }))
          ];
          break;
        case 'narrators':
          submenuItemsData = (data.narrators || []).map(n => ({
            label: n,
            value: `narrators.${btoa(unescape(encodeURIComponent(n)))}`
          }));
          break;
        case 'genres':
          submenuItemsData = (data.genres || []).map(g => ({
            label: g,
            value: `genres.${btoa(unescape(encodeURIComponent(g)))}`
          }));
          break;
        case 'tags':
          submenuItemsData = (data.tags || []).map(t => ({
            label: t,
            value: `tags.${btoa(unescape(encodeURIComponent(t)))}`
          }));
          break;
        case 'publishers':
          submenuItemsData = (data.publishers || []).map(p => ({
            label: p,
            value: `publishers.${btoa(unescape(encodeURIComponent(p)))}`
          }));
          break;
        case 'languages':
          submenuItemsData = (data.languages || []).map(l => ({
            label: l,
            value: `languages.${btoa(unescape(encodeURIComponent(l)))}`
          }));
          break;
        case 'decades':
          submenuItemsData = (data.publishedDecades || []).map(d => ({
            label: `${d}s`,
            value: `decades.${btoa(d)}`
          }));
          break;
        case 'duration':
          submenuItemsData = [
            { label: 'Under 1 Hour', value: 'duration.under-1h' },
            { label: '1 - 5 Hours', value: 'duration.1h-5h' },
            { label: '5 - 10 Hours', value: 'duration.5h-10h' },
            { label: 'Over 10 Hours', value: 'duration.over-10h' }
          ];
          break;
        case 'missing':
          submenuItemsData = [
            { label: 'Missing / Invalid', value: 'missing' }
          ];
          break;
      }

      if (submenuItemsData.length > 6) {
        if (searchContainer) searchContainer.classList.remove('hidden');
        if (searchInput) {
          searchInput.value = '';
        }
      } else {
        if (searchContainer) searchContainer.classList.add('hidden');
      }

      renderSubmenuItems();

      const rect = btnEl.getBoundingClientRect();
      const parentRect = btnEl.offsetParent.getBoundingClientRect();
      const relativeTop = rect.top - parentRect.top;
      submenu.style.top = `${relativeTop}px`;

      submenu.classList.remove('hidden');

      if (submenuItemsData.length > 6 && searchInput) {
        searchInput.focus();
      }
    };

    const renderFilterMenu = async () => {
      const activeLibId = getActiveLibraryId();
      if (!activeLibId) return;

      filterMenu.innerHTML = `
        <div class="px-3 py-2 text-xs text-black-200 flex items-center justify-center space-x-1">
          <svg class="animate-spin h-4 w-4 text-accent" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <span>Loading...</span>
        </div>
      `;

      try {
        if (!cachedFilterData || cachedFilterData.libraryId !== activeLibId) {
          const data = await request('GET', `/api/libraries/${activeLibId}/filterdata`);
          data.libraryId = activeLibId;
          cachedFilterData = data;
        }

        const data = cachedFilterData;
        const currentActiveFilter = localStorage.getItem('library-filterBy') || '';

        updateFilterLabel(currentActiveFilter, data);

        let menuHtml = `
          <button class="filter-option-btn w-full text-left px-3 py-1.5 text-xs text-black-50 hover:bg-black-500 hover:text-white flex items-center justify-between transition-colors focus:outline-none" data-value="">
            <span>Clear Filter</span>
            <span class="material-symbols text-[14px] check-icon ${!currentActiveFilter ? '' : 'hidden'}">check</span>
          </button>
          <div class="h-[1px] bg-black-400/40 my-1"></div>
          <div class="px-3 py-1 text-[10px] font-bold text-black-300 uppercase tracking-wider">Filter By</div>
        `;

        const categories = [
          { key: 'progress', label: 'Progress State' },
          { key: 'authors', label: 'Author', count: data.authors?.length || 0 },
          { key: 'series', label: 'Series', count: data.series?.length || 0 },
          { key: 'narrators', label: 'Narrator', count: data.narrators?.length || 0 },
          { key: 'genres', label: 'Genre', count: data.genres?.length || 0 },
          { key: 'tags', label: 'Tag', count: data.tags?.length || 0 },
          { key: 'publishers', label: 'Publisher', count: data.publishers?.length || 0 },
          { key: 'languages', label: 'Language', count: data.languages?.length || 0 },
          { key: 'decades', label: 'Decade', count: data.publishedDecades?.length || 0 },
          { key: 'duration', label: 'Duration' },
          { key: 'missing', label: 'Issues', count: data.numIssues || 0 }
        ];

        categories.forEach(cat => {
          if (cat.count === 0 && cat.key !== 'progress' && cat.key !== 'duration' && cat.key !== 'missing') return;

          const isActiveCat = currentActiveFilter && currentActiveFilter.startsWith(cat.key + '.');
          const isMissingActive = cat.key === 'missing' && currentActiveFilter === 'missing';
          const highlightClass = (isActiveCat || isMissingActive) ? 'text-accent font-medium' : '';

          menuHtml += `
            <button class="filter-cat-row-btn w-full text-left px-3 py-1.5 text-xs text-black-50 hover:bg-black-500 hover:text-white flex items-center justify-between transition-colors focus:outline-none ${highlightClass}" data-cat="${cat.key}">
              <span>${cat.label}</span>
              <span class="material-symbols text-[14px] text-black-200">chevron_right</span>
            </button>
          `;
        });

        filterMenu.innerHTML = menuHtml;

        filterMenu.querySelectorAll('.filter-cat-row-btn').forEach(btn => {
          const cat = btn.getAttribute('data-cat');
          btn.onmouseenter = () => openSubmenu(cat, data, btn);
          btn.onclick = (e) => {
            e.stopPropagation();
            openSubmenu(cat, data, btn);
          };
        });

        const clearBtn = filterMenu.querySelector('.filter-option-btn');
        if (clearBtn) {
          clearBtn.onclick = (e) => {
            e.stopPropagation();
            localStorage.setItem('library-filterBy', '');
            updateFilterLabel('', data);
            closeFilter();
            closeSubmenu();
            const activeLibId = getActiveLibraryId();
            if (activeLibId) loadDashboard(activeLibId);
          };
        }
      } catch (err) {
        console.error('Failed to load filter data:', err);
        filterMenu.innerHTML = `
          <div class="px-3 py-2 text-xs text-red-500">Failed to load filters</div>
        `;
      }
    };

    if (submenu) {
      submenu.onclick = (e) => e.stopPropagation();
    }
    if (searchInput) {
      searchInput.oninput = (e) => renderSubmenuItems(e.target.value);
    }

    const initialActiveLibId = getActiveLibraryId();
    if (initialActiveLibId) {
      request('GET', `/api/libraries/${initialActiveLibId}/filterdata`)
        .then(data => {
          data.libraryId = initialActiveLibId;
          cachedFilterData = data;
          updateFilterLabel(activeFilter, data);
        })
        .catch(err => console.error('Failed to load initial filter data:', err));
    } else {
      updateFilterLabel(activeFilter);
    }

    // Update Sort UI elements
    const sortLabels = {
      "media.metadata.title": "Sort: Title",
      "media.metadata.authorName": "Sort: Author",
      "media.metadata.publishedYear": "Sort: Year",
      "addedAt": "Sort: Date Added",
      "media.duration": "Sort: Duration",
      "random": "Sort: Random"
    };

    const updateSortLabel = (val) => {
      const labelEl = document.getElementById('sort-selected-label');
      if (labelEl) labelEl.textContent = sortLabels[val] || 'Sort: Title';
      
      sortMenu.querySelectorAll('.sort-option-btn').forEach(btn => {
        const check = btn.querySelector('.check-icon');
        if (btn.getAttribute('data-value') === val) {
          check?.classList.remove('hidden');
          btn.classList.add('text-accent', 'font-medium');
        } else {
          check?.classList.add('hidden');
          btn.classList.remove('text-accent', 'font-medium');
        }
      });
    };

    updateSortLabel(activeSort);

    sortMenu.querySelectorAll('.sort-option-btn').forEach(btn => {
      btn.onclick = (e) => {
        e.stopPropagation();
        const val = btn.getAttribute('data-value');
        localStorage.setItem('library-sortBy', val);
        updateSortLabel(val);
        closeSort();
        const activeLibId = getActiveLibraryId();
        if (activeLibId) loadDashboard(activeLibId);
      };
    });
  };

  initCustomFilterAndSort();
  initSearchPresets();

  const sortOrderToggle = document.getElementById('sort-order-toggle-btn');
  if (sortOrderToggle) {
    const persistedDesc = localStorage.getItem('library-sortDesc') === 'true';
    const icon = document.getElementById('sort-order-icon');
    if (icon) {
      icon.textContent = persistedDesc ? 'arrow_downward' : 'arrow_upward';
    }
    sortOrderToggle.onclick = () => {
      const icon = document.getElementById('sort-order-icon');
      if (icon) {
        const isDesc = icon.textContent === 'arrow_downward';
        icon.textContent = isDesc ? 'arrow_upward' : 'arrow_downward';
        localStorage.setItem('library-sortDesc', (!isDesc).toString());
      }
      const activeLibId = getActiveLibraryId();
      if (activeLibId) loadDashboard(activeLibId);
    };
  }

  // Style switcher initialization
  const styleBtnShelf = document.getElementById('style-btn-shelf');
  const styleBtnGrid = document.getElementById('style-btn-grid');
  const styleBtnList = document.getElementById('style-btn-list');

  const updateStyleSwitcherUI = (activeStyle) => {
    [styleBtnShelf, styleBtnGrid, styleBtnList].forEach(btn => {
      if (!btn) return;
      btn.classList.remove('text-accent', 'bg-black-500');
      btn.classList.add('text-black-100');
    });

    let activeBtn = styleBtnShelf;
    if (activeStyle === 'grid') activeBtn = styleBtnGrid;
    else if (activeStyle === 'list') activeBtn = styleBtnList;

    if (activeBtn) {
      activeBtn.classList.remove('text-black-100');
      activeBtn.classList.add('text-accent', 'bg-black-500');
    }
  };

  const currentStyle = localStorage.getItem('library-style') || 'shelf';
  updateStyleSwitcherUI(currentStyle);

  const setStyle = (newStyle) => {
    localStorage.setItem('library-style', newStyle);
    updateStyleSwitcherUI(newStyle);

    const shelfSizeCtrl = document.getElementById('shelf-size-control');
    if (shelfSizeCtrl) {
      let relPath = window.location.pathname;
      if (typeof ROUTER_BASE_PATH !== 'undefined' && ROUTER_BASE_PATH && relPath.startsWith(ROUTER_BASE_PATH)) {
        relPath = relPath.substring(ROUTER_BASE_PATH.length);
      }
      if (!relPath.startsWith('/')) {
        relPath = '/' + relPath;
      }
      const showControls = (relPath === '/' || relPath === '/library' || relPath === '/series' || relPath === '/authors' || relPath === '/collections' || relPath === '/playlists' || relPath === '/narrators');
      if (showControls && newStyle !== 'list') {
        shelfSizeCtrl.classList.remove('hidden');
      } else {
        shelfSizeCtrl.classList.add('hidden');
      }
    }

    const activeLibId = getActiveLibraryId();
    if (activeLibId) loadDashboard(activeLibId);
  };

  if (styleBtnShelf) styleBtnShelf.onclick = () => setStyle('shelf');
  if (styleBtnGrid) styleBtnGrid.onclick = () => setStyle('grid');
  if (styleBtnList) styleBtnList.onclick = () => setStyle('list');
}

function bootstrapApp(payload) {
  // Apply initial theme and custom CSS from login/authorization payload if available
  if (payload && payload.serverSettings) {
    window.serverSettings = payload.serverSettings;
    applyServerThemeAndCss(payload.serverSettings);
  }

  // On bootstrap, fetch server settings (GET /api/settings) and save to window.serverSettings
  request('GET', '/api/settings')
    .then(settings => {
      window.serverSettings = settings;
      applyServerThemeAndCss(settings);
    })
    .catch(err => {
      console.warn('Could not fetch server settings:', err);
    });

  // Populate User Identity Details
  const user = payload.user || {};
  window.currentUser = user;
  const userInitials = document.getElementById('user-initials');
  const userMenuUsername = document.getElementById('user-menu-username');
  const userDisplayName = document.getElementById('user-display-name');
  const userDisplayRole = document.getElementById('user-display-role');
  
  if (userInitials) {
    userInitials.textContent = (user.username || 'U').substring(0, 2).toUpperCase();
  }
  if (userMenuUsername) {
    userMenuUsername.textContent = user.username || 'User';
  }
  if (userDisplayName) {
    userDisplayName.textContent = user.username || 'User';
  }
  if (userDisplayRole) {
    userDisplayRole.textContent = user.type || 'User';
  }

  // Wire header activity button
  const activityBtn = document.getElementById('header-activity-btn');
  if (activityBtn) {
    activityBtn.onclick = () => {
      navigateTo('/stats');
    };
  }

  // Setup Admin / Root only features
  if (user.type === 'root' || user.type === 'admin') {
    const settingsBtn = document.getElementById('user-menu-settings-btn');
    const adminBtn = document.getElementById('user-menu-admin-btn');
    if (settingsBtn) settingsBtn.classList.remove('hidden');
    if (adminBtn) adminBtn.classList.remove('hidden');

    const headerSettingsBtn = document.getElementById('header-settings-btn');
    if (headerSettingsBtn) {
      headerSettingsBtn.classList.remove('hidden');
      headerSettingsBtn.onclick = () => {
        navigateTo('/settings');
      };
    }

    const batchToggleBtn = document.getElementById('batch-edit-toggle-btn');
    if (batchToggleBtn) batchToggleBtn.classList.remove('hidden');

    const scanBtn = document.getElementById('scan-library-btn');
    if (scanBtn) {
      scanBtn.classList.remove('hidden');
      scanBtn.onclick = async () => {
        const libId = getActiveLibraryId();
        if (!libId) return;
        try {
          await request('POST', `/api/libraries/${libId}/scan`);
          showToast('Scan requested successfully', 'success');
        } catch (err) {
          showToast('Failed to trigger scan: ' + err.message, 'error');
        }
      };
    }

    const headerUploadBtn = document.getElementById('header-upload-btn');
    if (headerUploadBtn) {
      headerUploadBtn.classList.remove('hidden');
      headerUploadBtn.onclick = () => {
        const libId = getActiveLibraryId();
        if (!libId) {
          showToast('No active library selected to upload to', 'warning');
          return;
        }
        import('./upload.js').then(module => {
          module.openUploadModal(libId);
        });
      };
    }

    const uploadBtn = document.getElementById('upload-btn');
    if (uploadBtn) {
      uploadBtn.classList.remove('hidden');
      uploadBtn.onclick = () => {
        const libId = getActiveLibraryId();
        if (!libId) {
          showToast('No active library selected to upload to', 'warning');
          return;
        }
        import('./upload.js').then(module => {
          module.openUploadModal(libId);
        });
      };
    }
  }

  // Set up global search input listener and dropdown menu handling
  const globalSearchInput = document.getElementById('global-search-input');
  const globalSearchClearBtn = document.getElementById('global-search-clear-btn');
  const globalSearchDropdown = document.getElementById('global-search-dropdown');
  const globalSearchResultsList = document.getElementById('global-search-results-list');
  const globalSearchContainer = document.getElementById('global-search-container');
  const mobileSearchBtn = document.getElementById('mobile-search-btn');

  if (globalSearchInput) {
    let searchDebounceTimeout = null;
    let lastQuery = '';
    let activeSearchResultIndex = -1;

    const getSelectableItems = () => {
      if (!globalSearchResultsList) return [];
      return globalSearchResultsList.querySelectorAll('li[data-type]');
    };

    const clearSearchResultHighlight = (items) => {
      items.forEach(item => {
        item.classList.remove('bg-black-300', 'text-white');
        item.classList.add('hover:bg-black-400', 'text-gray-50');
      });
    };

    const highlightSearchResult = (items, index) => {
      clearSearchResultHighlight(items);
      if (index >= 0 && index < items.length) {
        const item = items[index];
        item.classList.remove('hover:bg-black-400', 'text-gray-50');
        item.classList.add('bg-black-300', 'text-white');
        item.scrollIntoView({ block: 'nearest' });
      }
    };

    const hideSearchDropdown = () => {
      if (globalSearchDropdown) {
        globalSearchDropdown.classList.add('hidden');
      }
    };

    const showSearchDropdown = () => {
      if (globalSearchDropdown) {
        globalSearchDropdown.classList.remove('hidden');
      }
    };

    const updateSearchClearBtnVisibility = () => {
      const symbolEl = document.getElementById('global-search-icon-symbol');
      if (symbolEl) {
        if (globalSearchInput.value.length > 0) {
          symbolEl.textContent = 'close';
        } else {
          symbolEl.textContent = 'search';
        }
      }
    };

    if (mobileSearchBtn && globalSearchContainer) {
      mobileSearchBtn.onclick = (e) => {
        e.stopPropagation();
        globalSearchContainer.classList.add('mobile-active');
        globalSearchInput.focus();
        updateSearchClearBtnVisibility();
      };
    }

    // Close dropdown on click outside
    document.addEventListener('click', (e) => {
      const container = document.getElementById('global-search-container');
      const mobBtn = document.getElementById('mobile-search-btn');
      if (container && !container.contains(e.target) && (!mobBtn || !mobBtn.contains(e.target))) {
        hideSearchDropdown();
        if (container.classList.contains('mobile-active')) {
          container.classList.remove('mobile-active');
          updateSearchClearBtnVisibility();
        }
      }
    });

    // Handle clearing the search
    if (globalSearchClearBtn) {
      globalSearchClearBtn.onclick = () => {
        if (globalSearchInput.value === '') {
          if (globalSearchContainer && globalSearchContainer.classList.contains('mobile-active')) {
            globalSearchContainer.classList.remove('mobile-active');
            hideSearchDropdown();
            updateSearchClearBtnVisibility();
            return;
          }
        }
        globalSearchInput.value = '';
        updateSearchClearBtnVisibility();
        hideSearchDropdown();
        lastQuery = '';
        activeSearchResultIndex = -1;
        const activeLibId = getActiveLibraryId();
        if (activeLibId && (window.location.pathname === '/' || window.location.pathname === '/library')) {
          loadDashboard(activeLibId);
        }
        globalSearchInput.focus();
      };
    }

    // Function to escape HTML
    const escapeHtml = (str) => {
      if (!str) return '';
      return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
    };

    // Render search results
    const renderSearchResults = (data, query) => {
      if (!globalSearchResultsList) return;
      globalSearchResultsList.innerHTML = '';
      activeSearchResultIndex = -1;

      let totalResults = 0;
      const token = localStorage.getItem('token');

      // Helper to add results section
      const appendSection = (title, items, renderFunc) => {
        if (!items || items.length === 0) return;
        totalResults += items.length;

        const headerLi = document.createElement('li');
        headerLi.className = 'px-3 py-1 text-xs font-semibold text-gray-400 uppercase tracking-wider select-none bg-black-700/50';
        headerLi.textContent = title;
        globalSearchResultsList.appendChild(headerLi);

        items.forEach(item => {
          const li = document.createElement('li');
          li.className = 'px-3 py-2 hover:bg-black-400 cursor-pointer flex items-center text-gray-50 border-b border-black-600/30';
          renderFunc(li, item);
          globalSearchResultsList.appendChild(li);
        });
      };

      // 1. Books
      appendSection('Books', data.book, (li, b) => {
        const item = b.libraryItem || {};
        const media = item.media || {};
        const title = media.metadata?.title || item.title || 'Untitled';
        const subtitle = media.metadata?.subtitle || '';
        const authorName = media.metadata?.authorName || 'Unknown';
        const ts = item.updatedAt || item.addedAt || Date.now();
        const coverUrl = resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);

        li.setAttribute('data-type', 'item');
        li.setAttribute('data-id', item.id);
        li.innerHTML = `
          <img src="${coverUrl}" class="w-8 h-12 object-cover rounded-sm mr-3 bg-black-700" onerror="this.onerror=null; this.src='assets/images/logo.png'">
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(title)}</p>
            ${subtitle ? `<p class="truncate text-xs text-gray-300">${escapeHtml(subtitle)}</p>` : ''}
            <p class="truncate text-xs text-gray-400">${escapeHtml(authorName)}</p>
          </div>
        `;
      });

      // 2. Podcasts
      appendSection('Podcasts', data.podcast, (li, p) => {
        const item = p.libraryItem || {};
        const media = item.media || {};
        const title = media.metadata?.title || item.title || 'Untitled';
        const authorName = media.metadata?.author || 'Unknown';
        const ts = item.updatedAt || item.addedAt || Date.now();
        const coverUrl = resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);

        li.setAttribute('data-type', 'item');
        li.setAttribute('data-id', item.id);
        li.innerHTML = `
          <img src="${coverUrl}" class="w-8 h-12 object-cover rounded-sm mr-3 bg-black-700" onerror="this.onerror=null; this.src='assets/images/logo.png'">
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(title)}</p>
            <p class="truncate text-xs text-gray-400">${escapeHtml(authorName)}</p>
          </div>
        `;
      });

      // 3. Episodes
      appendSection('Episodes', data.episodes, (li, ep) => {
        const item = ep.libraryItem || {};
        const media = item.media || {};
        const episodeTitle = ep.title || 'No Title';
        const podcastTitle = media.metadata?.title || 'No Title';
        const ts = item.updatedAt || item.addedAt || Date.now();
        const coverUrl = resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);

        li.setAttribute('data-type', 'item');
        li.setAttribute('data-id', item.id);
        li.innerHTML = `
          <img src="${coverUrl}" class="w-8 h-12 object-cover rounded-sm mr-3 bg-black-700" onerror="this.onerror=null; this.src='assets/images/logo.png'">
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(episodeTitle)}</p>
            <p class="truncate text-xs text-gray-400">${escapeHtml(podcastTitle)}</p>
          </div>
        `;
      });

      // 4. Authors
      appendSection('Authors', data.authors, (li, auth) => {
        const authorInitials = auth.name ? auth.name.split(' ').map(n => n[0]).join('').substring(0, 2).toUpperCase() : '';
        const authorImageUrl = resolvePath(`/api/authors/${auth.id}/image?token=${token}`);

        li.setAttribute('data-type', 'author');
        li.setAttribute('data-id', auth.id);
        li.innerHTML = `
          <img src="${authorImageUrl}" class="w-8 h-8 rounded-full object-cover mr-3 bg-black-700" onerror="this.onerror=null; this.style.display='none'; this.nextElementSibling.style.display='flex';">
          <div class="w-8 h-8 rounded-full bg-accent text-primary font-bold text-xs flex items-center justify-center mr-3 select-none hidden">${escapeHtml(authorInitials)}</div>
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(auth.name)}</p>
            <p class="truncate text-xs text-gray-400">${auth.numBooks} ${auth.numBooks === 1 ? 'Book' : 'Books'}</p>
          </div>
        `;
      });

      // 5. Series
      appendSection('Series', data.series, (li, ser) => {
        li.setAttribute('data-type', 'series');
        li.setAttribute('data-id', ser.id);
        li.innerHTML = `
          <div class="w-8 h-8 flex items-center justify-center mr-3 bg-black-600 rounded-sm select-none">
            <span class="material-symbols text-xl text-gray-200">layers</span>
          </div>
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(ser.name)}</p>
          </div>
        `;
      });

      // 6. Narrators
      appendSection('Narrators', data.narrators, (li, narr) => {
        li.setAttribute('data-type', 'narrator');
        li.setAttribute('data-val', narr.name);
        li.innerHTML = `
          <div class="w-8 h-8 flex items-center justify-center mr-3 select-none">
            <span class="material-symbols text-xl text-gray-200">record_voice_over</span>
          </div>
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(narr.name)}</p>
            <p class="truncate text-xs text-gray-400">${narr.numBooks} ${narr.numBooks === 1 ? 'Book' : 'Books'}</p>
          </div>
        `;
      });

      // 7. Tags
      appendSection('Tags', data.tags, (li, tag) => {
        li.setAttribute('data-type', 'tag');
        li.setAttribute('data-val', tag.name);
        li.innerHTML = `
          <div class="w-8 h-8 flex items-center justify-center mr-3 select-none">
            <span class="material-symbols text-xl text-gray-200">local_offer</span>
          </div>
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(tag.name)}</p>
            <p class="truncate text-xs text-gray-400">${tag.numItems} ${tag.numItems === 1 ? 'Item' : 'Items'}</p>
          </div>
        `;
      });

      // 8. Genres
      appendSection('Genres', data.genres, (li, gen) => {
        li.setAttribute('data-type', 'genre');
        li.setAttribute('data-val', gen.name);
        li.innerHTML = `
          <div class="w-8 h-8 flex items-center justify-center mr-3 select-none">
            <span class="material-symbols text-xl text-gray-200">category</span>
          </div>
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(gen.name)}</p>
            <p class="truncate text-xs text-gray-400">${gen.numItems} ${gen.numItems === 1 ? 'Item' : 'Items'}</p>
          </div>
        `;
      });

      if (totalResults === 0) {
        globalSearchResultsList.innerHTML = `<li class="text-center py-4 text-gray-400 select-none">No results found</li>`;
      }

      // Wire up clicks on items
      globalSearchResultsList.querySelectorAll('li[data-type]').forEach(el => {
        el.onclick = (e) => {
          e.stopPropagation();
          const type = el.getAttribute('data-type');
          hideSearchDropdown();

          if (type === 'item') {
            const id = el.getAttribute('data-id');
            navigateTo(`/item/${id}`);
          } else if (type === 'author') {
            const id = el.getAttribute('data-id');
            navigateTo(`/author/${id}`);
          } else if (type === 'series') {
            const id = el.getAttribute('data-id');
            navigateTo(`/series/${id}`);
          } else if (type === 'tag') {
            const val = el.getAttribute('data-val');
            const encoded = btoa(unescape(encodeURIComponent(val)));
            localStorage.setItem('library-filterBy', 'tags.' + encoded);
            const labelEl = document.getElementById('filter-selected-label');
            if (labelEl) labelEl.textContent = 'Tag: ' + val;
            navigateTo('/');
          } else if (type === 'genre') {
            const val = el.getAttribute('data-val');
            const encoded = btoa(unescape(encodeURIComponent(val)));
            localStorage.setItem('library-filterBy', 'genres.' + encoded);
            const labelEl = document.getElementById('filter-selected-label');
            if (labelEl) labelEl.textContent = 'Genre: ' + val;
            navigateTo('/');
          } else if (type === 'narrator') {
            const val = el.getAttribute('data-val');
            const encoded = btoa(unescape(encodeURIComponent(val)));
            localStorage.setItem('library-filterBy', 'narrators.' + encoded);
            const labelEl = document.getElementById('filter-selected-label');
            if (labelEl) labelEl.textContent = 'Narrator: ' + val;
            navigateTo('/');
          }
        };
      });
    };

    globalSearchInput.onfocus = () => {
      const query = globalSearchInput.value.trim();
      if (query) {
        showSearchDropdown();
      }
    };

    globalSearchInput.oninput = (e) => {
      const query = globalSearchInput.value.trim();
      
      if (!query) {
        updateSearchClearBtnVisibility();
        hideSearchDropdown();
        lastQuery = '';
        const activeLibId = getActiveLibraryId();
        if (activeLibId && (window.location.pathname === '/' || window.location.pathname === '/library')) {
          loadDashboard(activeLibId);
        }
        return;
      }

      updateSearchClearBtnVisibility();
      showSearchDropdown();

      if (globalSearchResultsList) {
        // Show spinner
        globalSearchResultsList.innerHTML = `<li class="text-center py-4 text-gray-400 select-none"><span class="material-symbols animate-spin">sync</span></li>`;
      }

      clearTimeout(searchDebounceTimeout);
      searchDebounceTimeout = setTimeout(async () => {
        if (query !== globalSearchInput.value.trim()) return;
        lastQuery = query;

        const activeLibId = getActiveLibraryId();
        if (!activeLibId) return;

        try {
          const response = await request('GET', `/api/libraries/${activeLibId}/search?q=${encodeURIComponent(query)}&limit=3`);
          if (query === globalSearchInput.value.trim()) {
            renderSearchResults(response, query);
          }
        } catch (err) {
          console.error('Search error:', err);
          if (query === globalSearchInput.value.trim()) {
            globalSearchResultsList.innerHTML = `<li class="text-center py-4 text-error select-none">Error loading results</li>`;
          }
        }

        // Keep real-time dashboard filtering in sync if currently on dashboard
        let relPath = window.location.pathname;
        if (typeof ROUTER_BASE_PATH !== 'undefined' && ROUTER_BASE_PATH && relPath.startsWith(ROUTER_BASE_PATH)) {
          relPath = relPath.substring(ROUTER_BASE_PATH.length);
        }
        if (!relPath.startsWith('/')) {
          relPath = '/' + relPath;
        }
        const isDashboard = (relPath === '/' || relPath === '/library');
        if (isDashboard) {
          loadDashboard(activeLibId);
        }
      }, 300);
    };

    // Keyboard support (Arrow keys, ENTER, ESCAPE)
    globalSearchInput.onkeydown = (e) => {
      const items = getSelectableItems();
      
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        if (items.length === 0) return;
        activeSearchResultIndex = (activeSearchResultIndex + 1) % items.length;
        highlightSearchResult(items, activeSearchResultIndex);
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        if (items.length === 0) return;
        activeSearchResultIndex = (activeSearchResultIndex - 1 + items.length) % items.length;
        highlightSearchResult(items, activeSearchResultIndex);
      } else if (e.key === 'Enter') {
        if (activeSearchResultIndex >= 0 && activeSearchResultIndex < items.length) {
          e.preventDefault();
          items[activeSearchResultIndex].click();
        } else {
          e.preventDefault();
          hideSearchDropdown();
          const activeLibId = getActiveLibraryId();
          let relPath = window.location.pathname;
          if (typeof ROUTER_BASE_PATH !== 'undefined' && ROUTER_BASE_PATH && relPath.startsWith(ROUTER_BASE_PATH)) {
            relPath = relPath.substring(ROUTER_BASE_PATH.length);
          }
          if (!relPath.startsWith('/')) {
            relPath = '/' + relPath;
          }
          const isDashboard = (relPath === '/' || relPath === '/library');
          if (!isDashboard) {
            navigateTo('/');
          } else if (activeLibId) {
            loadDashboard(activeLibId);
          }
        }
      } else if (e.key === 'Escape') {
        e.preventDefault();
        hideSearchDropdown();
        globalSearchInput.blur();
      }
    };
  }

  // Initialize library dropdown
  initLibrary(payload);

  // Transition to main view
  showAppContainer();

  // Connect websocket and listen to events
  const token = localStorage.getItem('token');
  if (token) {
    connectSocket(token);
    
    // Register listener for progress syncing across devices
    onEvent('user_item_progress_updated', (data) => {
      console.log('[Socket] progress updated:', data);
      const activeLibId = getActiveLibraryId();
      if (activeLibId && isDashboardActive()) {
        loadDashboard(activeLibId);
      }
    });

    onEvent('user_updated', (data) => {
      console.log('[Socket] user updated:', data);
      const activeLibId = getActiveLibraryId();
      if (activeLibId && isDashboardActive()) {
        loadDashboard(activeLibId);
      }
    });

    // Scan WebSocket Listeners
    onEvent('library_scan_started', (libraryId) => {
      if (libraryId === getActiveLibraryId()) {
        const icon = document.getElementById('scan-btn-icon');
        if (icon) icon.classList.add('animate-spin');
        showToast('Library scan started', 'info');
      }
    });

    onEvent('library_scan_complete', (libraryId) => {
      if (libraryId === getActiveLibraryId()) {
        const icon = document.getElementById('scan-btn-icon');
        if (icon) icon.classList.remove('animate-spin');
        showToast('Library scan completed', 'success');
        if (isDashboardActive()) {
          loadDashboard(libraryId); // refresh
        }
      }
    });
  }

  // Update sidebar item visibility on initial load
  updateSidebarVisibility();

  // Route to the current URL path on bootstrap
  navigateTo(window.location.pathname, false);
}

function navigateTo(path, pushState = true) {
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

  const filterBtn = document.getElementById('filter-dropdown-btn');
  const sortBtn = document.getElementById('sort-dropdown-btn');
  const sortOrderToggle = document.getElementById('sort-order-toggle-btn');
  const shelfSizeControl = document.getElementById('shelf-size-control');
  const styleSwitcher = document.getElementById('style-switcher');

  const showControls = (relPath === '/' || relPath === '/library');
  const showShelfSize = (relPath === '/' || relPath === '/library' || relPath === '/series' || relPath === '/authors' || relPath === '/collections' || relPath === '/playlists' || relPath === '/narrators');
  
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
    if (showShelfSize && currentStyle !== 'list') shelfSizeControl.classList.remove('hidden');
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

  if (relPath === '/' || relPath === '/library') {
    const isMissing = localStorage.getItem('library-filterBy') === 'missing';
    if (isMissing) {
      highlightSidebarLink('Issues');
    } else {
      highlightSidebarLink('Home');
    }
    const viewTitle = document.getElementById('view-title');
    if (viewTitle) viewTitle.textContent = isMissing ? 'Issues' : 'Home';
    if (activeLibId) {
      loadDashboard(activeLibId);
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
      l.classList.remove('bg-primary/80', 'text-white');
      l.classList.add('hover:bg-primary', 'text-white/80', 'bg-bg/60');
      const activeBar = l.querySelector('.active-indicator');
      if (activeBar) activeBar.classList.add('hidden');
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
    if (activeLibId) loadDashboard(activeLibId);
  }
}
window.navigateTo = navigateTo;

export function showToast(message, type = 'info') {
  const container = document.getElementById('toast-container');
  if (!container) return;
  
  const toast = document.createElement('div');
  toast.className = 'px-4 py-2.5 rounded shadow-lg text-sm transition-all duration-300 transform translate-y-2 opacity-0 flex items-center space-x-2 ';
  
  if (type === 'success') {
    toast.className += 'bg-emerald-800 border border-emerald-500 text-emerald-100';
  } else if (type === 'error') {
    toast.className += 'bg-red-950 border border-red-500 text-red-100';
  } else {
    toast.className += 'bg-primary border border-black-300 text-white';
  }
  
  const iconName = type === 'success' ? 'check_circle' : type === 'error' ? 'error' : 'info';
  toast.innerHTML = `
    <span class="material-symbols text-lg">${iconName}</span>
    <span>${message}</span>
  `;
  
  container.appendChild(toast);
  
  setTimeout(() => {
    toast.classList.remove('translate-y-2', 'opacity-0');
  }, 10);
  
  setTimeout(() => {
    toast.classList.add('translate-y-2', 'opacity-0');
    setTimeout(() => {
      toast.remove();
    }, 300);
  }, 4000);
}

function showNoLibrariesWelcome() {
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

function isDashboardActive() {
  const activeLink = document.querySelector('#siderail-buttons-container a.bg-primary\\/80');
  if (!activeLink) return false;
  const pageName = activeLink.querySelector('p').textContent.trim();
  const hasDetailsBtn = !!document.getElementById('details-back-btn');
  return (pageName === 'Home' || pageName === 'Library') && !hasDetailsBtn;
}

window.formatDateTime = function(dateStr) {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return dateStr;

  // Get format preferences with fallbacks
  const dateFormat = (window.serverSettings && window.serverSettings.dateFormat) || 'MM/DD/YYYY';
  const timeFormat = (window.serverSettings && window.serverSettings.timeFormat) || 'HH:mm';

  // Format Date parts
  const yyyy = date.getFullYear();
  const mm = String(date.getMonth() + 1).padStart(2, '0');
  const dd = String(date.getDate()).padStart(2, '0');

  let datePart = '';
  if (dateFormat === 'YYYY-MM-DD') {
    datePart = `${yyyy}-${mm}-${dd}`;
  } else if (dateFormat === 'DD/MM/YYYY') {
    datePart = `${dd}/${mm}/${yyyy}`;
  } else {
    datePart = `${mm}/${dd}/${yyyy}`;
  }

  // Format Time parts
  const hours = date.getHours();
  const minutes = String(date.getMinutes()).padStart(2, '0');

  let timePart = '';
  if (timeFormat === 'h:mm A') {
    const ampm = hours >= 12 ? 'PM' : 'AM';
    const displayHours = hours % 12 || 12;
    timePart = `${displayHours}:${minutes} ${ampm}`;
  } else {
    const displayHours = String(hours).padStart(2, '0');
    timePart = `${displayHours}:${minutes}`;
  }

  return `${datePart} ${timePart}`;
};

// Global Drag & Drop Handler for file/folder upload
let dragOverlay = null;

const showDragOverlay = () => {
  if (dragOverlay) return;
  dragOverlay = document.createElement('div');
  dragOverlay.className = 'fixed inset-0 bg-accent/10 border-4 border-dashed border-accent z-50 flex flex-col items-center justify-center pointer-events-none backdrop-blur-[2px] transition-all';
  dragOverlay.innerHTML = `
    <div class="bg-primary/95 border border-black-300 rounded-lg p-6 shadow-2xl flex flex-col items-center max-w-sm text-center">
      <span class="material-symbols text-5xl text-accent mb-2 animate-bounce">cloud_upload</span>
      <h3 class="text-lg font-bold text-white mb-1">Upload to Audiobookshelf</h3>
      <p class="text-xs text-black-100">Drop files or folders here to add them to your active library.</p>
    </div>
  `;
  document.body.appendChild(dragOverlay);
};

const hideDragOverlay = () => {
  if (dragOverlay) {
    dragOverlay.remove();
    dragOverlay = null;
  }
};

window.addEventListener('dragover', (e) => {
  const token = localStorage.getItem('token');
  const userJson = localStorage.getItem('user');
  let user = null;
  try { user = userJson ? JSON.parse(userJson) : null; } catch(_) {}
  if (!token || !user || (user.type !== 'root' && user.type !== 'admin')) return;

  e.preventDefault();
  showDragOverlay();
});

window.addEventListener('dragleave', (e) => {
  if (e.clientX <= 0 || e.clientY <= 0 || e.clientX >= window.innerWidth || e.clientY >= window.innerHeight) {
    hideDragOverlay();
  }
});

async function getFilesFromEntry(entry, path = '') {
  if (entry.isFile) {
    const file = await new Promise((resolve, reject) => entry.file(resolve, reject));
    return [{ file, path: path + file.name }];
  } else if (entry.isDirectory) {
    const dirReader = entry.createReader();
    const entries = await new Promise((resolve, reject) => {
      dirReader.readEntries(resolve, reject);
    });
    const filePromises = entries.map(childEntry => 
      getFilesFromEntry(childEntry, path + entry.name + '/')
    );
    const results = await Promise.all(filePromises);
    return results.flat();
  }
  return [];
}

window.addEventListener('drop', async (e) => {
  const token = localStorage.getItem('token');
  const userJson = localStorage.getItem('user');
  let user = null;
  try { user = userJson ? JSON.parse(userJson) : null; } catch(_) {}
  if (!token || !user || (user.type !== 'root' && user.type !== 'admin')) return;

  e.preventDefault();
  hideDragOverlay();

  const libId = getActiveLibraryId();
  if (!libId) {
    showToast('No active library selected to upload to', 'warning');
    return;
  }

  const items = e.dataTransfer.items;
  if (!items || items.length === 0) return;

  const fileEntries = [];
  for (let i = 0; i < items.length; i++) {
    const entry = items[i].webkitGetAsEntry();
    if (entry) {
      fileEntries.push(entry);
    }
  }

  if (fileEntries.length > 0) {
    showToast('Processing dropped items...', 'info');
    const filesList = [];
    for (const entry of fileEntries) {
      const files = await getFilesFromEntry(entry);
      filesList.push(...files);
    }
    
    if (filesList.length > 0) {
      import('./upload.js').then(module => {
        module.openUploadModal(libId, filesList);
      });
    }
  }
});


