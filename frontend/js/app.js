// js/app.js
import { navigateTo, highlightSidebarLink, updateSidebarVisibility, isDashboardActive } from './router.js';
import { showToast } from './toast.js';
export { showToast } from './toast.js';

import { initAuth, showAppContainer, logout } from './auth.js';
import { initLibrary, getActiveLibraryId, getActiveLibrary } from './library.js';
import { loadDashboard } from './dashboard.js';
import { request, resolvePath, ROUTER_BASE_PATH } from './api.js';
import { connectSocket, disconnectSocket, registerAppSocketListeners } from './socket.js';
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

export function updateSearchPlaceholder() {
  const activeLib = getActiveLibrary();
  const globalSearchInput = document.getElementById('global-search-input');
  if (globalSearchInput) {
    if (activeLib) {
      if (activeLib.mediaType === 'podcast') {
        globalSearchInput.placeholder = 'Search Podcasts...';
      } else {
        globalSearchInput.placeholder = 'Search Books...';
      }
    } else {
      globalSearchInput.placeholder = 'Search Library...';
    }
  }
}

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


function setupEventHandlers() {
  // Password Visibility Toggle
  document.querySelectorAll('.password-wrapper').forEach(wrapper => {
    const input = wrapper.querySelector('input');
    const toggleBtn = wrapper.querySelector('.password-toggle-btn');
    const icon = toggleBtn ? toggleBtn.querySelector('.material-symbols') : null;
    if (input && toggleBtn && icon) {
      toggleBtn.onclick = (e) => {
        e.preventDefault();
        e.stopPropagation();
        if (input.type === 'password') {
          input.type = 'text';
          icon.textContent = 'visibility_off';
        } else {
          input.type = 'password';
          icon.textContent = 'visibility';
        }
      };
    }
  });

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

      const authWarning = document.getElementById('login-auth-warning');
      if (authWarning) {
        authWarning.classList.add('hidden');
        authWarning.classList.remove('flex');
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
        let errorMsg = err.message;
        try {
          const parsed = JSON.parse(err.message);
          if (parsed && parsed.error) {
            errorMsg = parsed.error;
          }
        } catch (_) {}
        loginError.textContent = errorMsg || 'Invalid username or password';
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
    const ids = ['library-dropdown-menu', 'user-dropdown', 'filter-dropdown-menu', 'sort-dropdown-menu', 'header-notification-dropdown'];
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
      
      // Close mobile sidebar if open
      const sidebar = document.getElementById('sidebar');
      if (sidebar) {
        sidebar.classList.remove('open');
        const backdrop = document.getElementById('sidebar-backdrop');
        if (backdrop) {
          backdrop.classList.remove('show');
        }
        setTimeout(() => {
          if (!sidebar.classList.contains('open')) {
            sidebar.classList.add('hidden');
            sidebar.classList.remove('fixed', 'left-0', 'top-16', 'bottom-0', 'flex');
          }
          if (backdrop && !backdrop.classList.contains('show')) {
            backdrop.classList.add('hidden');
          }
        }, 200);
      }
      
      const pEl = link.querySelector('p');
      if (!pEl) return;
      const pageName = pEl.textContent.trim();
      let path = '/';
      if (pageName === 'Library') path = '/library';
      else if (pageName === 'Playlists') path = '/playlists';
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

  // Sidebar Footer Buttons Click Handler
  const sidebarHelpBtn = document.getElementById('sidebar-help-btn');
  if (sidebarHelpBtn) {
    sidebarHelpBtn.addEventListener('click', (e) => {
      e.preventDefault();
      window.open('https://www.audiobookshelf.org/docs', '_blank');
    });
  }

  const sidebarVersionBtn = document.getElementById('sidebar-version');
  if (sidebarVersionBtn) {
    sidebarVersionBtn.addEventListener('click', (e) => {
      e.preventDefault();
      window.open('https://github.com/advplyr/audiobookshelf/releases', '_blank');
    });
  }

  // Global Listeners for Modular Events
  window.addEventListener('auth-unauthorized', () => {
    disconnectSocket();
    logout();
  });

  window.addEventListener('library-changed', (e) => {
    const libraryId = e.detail.libraryId;
    if (!libraryId) return;
    
    updateSidebarVisibility();
    updateSearchPlaceholder();
    if (window.updateCustomSortMenu) {
      window.updateCustomSortMenu();
    }
    
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
      let pathName = window.location.pathname;
      if (typeof ROUTER_BASE_PATH !== 'undefined' && ROUTER_BASE_PATH && pathName.startsWith(ROUTER_BASE_PATH)) {
        pathName = pathName.substring(ROUTER_BASE_PATH.length);
      }
      if (!pathName.startsWith('/')) {
        pathName = '/' + pathName;
      }
      loadDashboard(libraryId, pathName === '/');
    }
  });

  window.addEventListener('navigate-to-dashboard', (e) => {
    const activeLibId = getActiveLibraryId();
    if (!activeLibId) return;
    const { filterBy, filterLabel } = e.detail;

    window.history.pushState(null, '', resolvePath('/library'));

    if (filterLabel === 'Issues') {
      highlightSidebarLink('Issues');
    } else {
      highlightSidebarLink('Library');
    }

    const viewTitle = document.getElementById('view-title');
    if (viewTitle) {
      viewTitle.textContent = filterLabel || 'Library';
    }

    loadDashboard(activeLibId, false, filterBy, filterLabel);
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
    const sliderInput = document.getElementById('shelf-size-slider');
    if (!decBtn || !incBtn || !valSpan) return;

    let currentSize = parseInt(localStorage.getItem('bookshelf-card-width')) || 120;
    currentSize = Math.max(80, Math.min(240, currentSize));

    const updateSize = (newSize) => {
      currentSize = Math.max(80, Math.min(240, newSize));
      localStorage.setItem('bookshelf-card-width', currentSize);
      valSpan.textContent = currentSize;
      if (sliderInput) {
        sliderInput.value = currentSize;
      }
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

    if (sliderInput) {
      sliderInput.oninput = (e) => {
        updateSize(parseInt(e.target.value));
      };
      sliderInput.onclick = (e) => {
        e.stopPropagation();
      };
    }
  };

  initShelfSizing();

  // Sorting/Filtering dropdown handlers (Custom animated dropdowns)
  const initCustomFilterAndSort = () => {
    const filterBtn = document.getElementById('filter-dropdown-btn');
    const filterMenu = document.getElementById('filter-dropdown-menu');
    const sortBtn = document.getElementById('sort-dropdown-btn');
    const sortMenu = document.getElementById('sort-dropdown-menu');

    if (!filterBtn || !filterMenu || !sortBtn || !sortMenu) return;

    filterBtn.setAttribute('aria-haspopup', 'menu');
    filterBtn.setAttribute('aria-expanded', 'false');
    sortBtn.setAttribute('aria-haspopup', 'menu');
    sortBtn.setAttribute('aria-expanded', 'false');
    filterMenu.setAttribute('role', 'menu');
    sortMenu.setAttribute('role', 'menu');

    // Toggles for Filter
    let filterOpen = false;
    filterMenu.classList.add('transition-all', 'duration-150', 'ease-out', 'transform', 'scale-95', 'opacity-0');
    
    const closeFilter = () => {
      if (!filterOpen) return;
      filterOpen = false;
      filterBtn.setAttribute('aria-expanded', 'false');
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
        filterBtn.setAttribute('aria-expanded', 'true');
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
      sortBtn.setAttribute('aria-expanded', 'false');
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
        sortBtn.setAttribute('aria-expanded', 'true');
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
      if (window.innerWidth < 640 && filterMenu && filterOpen) {
        filterMenu.classList.remove('hidden');
      }
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
      const clearBtn = document.getElementById('filter-clear-btn');
      if (clearBtn) {
        if (val) {
          clearBtn.classList.remove('hidden');
        } else {
          clearBtn.classList.add('hidden');
        }
      }
      const filterBtn = document.getElementById('filter-dropdown-btn');
      if (filterBtn) {
        if (val) {
          filterBtn.classList.add('text-accent');
          filterBtn.classList.remove('text-black-50');
        } else {
          filterBtn.classList.remove('text-accent');
          filterBtn.classList.add('text-black-50');
        }
      }
    };

    window.updateFilterLabelGlobal = (val) => {
      updateFilterLabel(val, cachedFilterData);
    };

    const renderSubmenuItems = (filterText = '') => {
      if (!submenuItems) return;
      const activeFilterVal = localStorage.getItem('library-filterBy') || '';
      const filtered = submenuItemsData.filter(item => 
        item.label.toLowerCase().includes(filterText.toLowerCase())
      );

      let itemsHtml = '';
      if (window.innerWidth < 640) {
        itemsHtml += `
          <button id="filter-submenu-back-btn" class="w-full text-left px-3 py-2 text-xs text-accent font-semibold hover:bg-black-400 flex items-center space-x-1.5 transition-colors border-b border-black-400/30 sticky top-0 bg-primary z-20 focus:outline-none">
            <span class="material-symbols text-sm">arrow_back</span>
            <span>Back to Categories</span>
          </button>
        `;
      }

      if (filtered.length === 0) {
        submenuItems.innerHTML = itemsHtml + `
          <div class="px-3 py-2 text-xs text-black-200">No items found</div>
        `;
        const backBtn = submenuItems.querySelector('#filter-submenu-back-btn');
        if (backBtn) {
          backBtn.onclick = (e) => {
            e.stopPropagation();
            closeSubmenu();
          };
        }
        return;
      }

      submenuItems.innerHTML = itemsHtml + filtered.map(item => {
        const isSelected = activeFilterVal === item.value;
        return `
          <button class="filter-submenu-option-btn w-full text-left px-3 py-1.5 text-xs text-black-50 hover:bg-black-400 hover:text-white flex items-center justify-between transition-colors focus:outline-none ${isSelected ? 'text-accent font-medium' : ''}" data-value="${item.value}">
            <span class="truncate pr-2">${item.label}</span>
            <span class="material-symbols text-[14px] check-icon ${isSelected ? '' : 'hidden'}">check</span>
          </button>
        `;
      }).join('');

      const backBtn = submenuItems.querySelector('#filter-submenu-back-btn');
      if (backBtn) {
        backBtn.onclick = (e) => {
          e.stopPropagation();
          closeSubmenu();
        };
      }

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
      submenu.setAttribute('role', 'menu');

      const rect = btnEl.getBoundingClientRect();
      const parentRect = btnEl.offsetParent.getBoundingClientRect();
      const relativeTop = rect.top - parentRect.top;

      // Dynamic horizontal positioning to prevent screen overflow on narrow viewports
      const submenuWidth = 224; // Width corresponding to w-56
      if (window.innerWidth < 640) {
        // Mobile view: overlay the main dropdown menu in place
        submenu.style.right = '0px';
        submenu.style.top = '100%';
        submenu.style.width = '176px';
        submenu.style.zIndex = '60';
        submenu.style.marginTop = '0.375rem'; // match mt-1.5
      } else if (rect.left - submenuWidth < 10) {
        submenu.style.right = '0px';
        submenu.style.top = `${relativeTop}px`;
        submenu.style.width = '224px';
        submenu.style.zIndex = '60';
        submenu.style.marginTop = '0px';
      } else {
        submenu.style.right = '182px';
        submenu.style.top = `${relativeTop}px`;
        submenu.style.width = '224px';
        submenu.style.zIndex = '50';
        submenu.style.marginTop = '0px';
      }

      submenu.classList.remove('hidden');
      if (window.innerWidth < 640 && filterMenu) {
        filterMenu.classList.add('hidden');
      }

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
          <button class="filter-option-btn w-full text-left px-3 py-1.5 text-xs text-black-50 hover:bg-black-400 hover:text-white flex items-center justify-between transition-colors focus:outline-none" data-value="">
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

          // Display count for categories if count is > 0
          const countBadge = (cat.count !== undefined && cat.count > 0) 
            ? ` <span class="text-black-200 text-[10px] font-normal">(${cat.count})</span>` 
            : '';

          menuHtml += `
            <button class="filter-cat-row-btn w-full text-left px-3 py-1.5 text-xs text-black-50 hover:bg-black-400 hover:text-white flex items-center justify-between transition-colors focus:outline-none ${highlightClass}" data-cat="${cat.key}">
              <span>${cat.label}${countBadge}</span>
              <span class="material-symbols text-[14px] text-black-200">chevron_right</span>
            </button>
          `;
        });

        filterMenu.innerHTML = menuHtml;

        // Auto-close submenu when entering any elements that are not category rows
        filterMenu.querySelectorAll(':scope > *:not(.filter-cat-row-btn)').forEach(el => {
          el.addEventListener('mouseenter', () => {
            closeSubmenu();
          });
        });

        filterMenu.querySelectorAll('.filter-cat-row-btn').forEach(btn => {
          const cat = btn.getAttribute('data-cat');
          btn.onmouseenter = () => openSubmenu(cat, data, btn);
          btn.onfocus = () => openSubmenu(cat, data, btn);
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
    const filterClearBtn = document.getElementById('filter-clear-btn');
    if (filterClearBtn) {
      filterClearBtn.onclick = (e) => {
        e.stopPropagation();
        localStorage.setItem('library-filterBy', '');
        updateFilterLabel('', cachedFilterData);
        closeFilter();
        closeSubmenu();
        const activeLibId = getActiveLibraryId();
        if (activeLibId) loadDashboard(activeLibId);
      };
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
      "media.metadata.authorNameLF": "Sort: Author (Last, First)",
      "media.metadata.author": "Sort: Publisher",
      "media.metadata.publishedYear": "Sort: Year",
      "addedAt": "Sort: Date Added",
      "birthtimeMs": "Sort: Date Created",
      "mtimeMs": "Sort: Date Modified",
      "sequence": "Sort: Sequence",
      "progress": "Sort: Progress",
      "media.duration": "Sort: Duration",
      "size": "Sort: Size",
      "random": "Sort: Random",
      "media.numTracks": "Sort: Episodes"
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

    const renderSortMenu = () => {
      const activeLib = getActiveLibrary();
      const mediaType = activeLib ? activeLib.mediaType : 'book';
      let currentSort = localStorage.getItem('library-sortBy') || 'media.metadata.title';

      const podcastAllowed = [
        'media.metadata.title',
        'media.metadata.author',
        'addedAt',
        'birthtimeMs',
        'mtimeMs',
        'progress',
        'media.numTracks',
        'size',
        'random'
      ];
      const bookAllowed = [
        'media.metadata.title',
        'media.metadata.authorName',
        'media.metadata.authorNameLF',
        'media.metadata.publishedYear',
        'addedAt',
        'birthtimeMs',
        'mtimeMs',
        'sequence',
        'progress',
        'media.duration',
        'size',
        'random'
      ];

      // Validate/map sort options based on mediaType
      if (mediaType === 'podcast') {
        if (currentSort === 'media.metadata.authorName') {
          currentSort = 'media.metadata.author';
        } else if (!podcastAllowed.includes(currentSort)) {
          currentSort = 'media.metadata.title';
        }
      } else {
        if (currentSort === 'media.metadata.author') {
          currentSort = 'media.metadata.authorName';
        } else if (!bookAllowed.includes(currentSort)) {
          currentSort = 'media.metadata.title';
        }
      }
      localStorage.setItem('library-sortBy', currentSort);

      let options = [];
      if (mediaType === 'podcast') {
        options = [
          { value: 'media.metadata.title', label: 'Title' },
          { value: 'media.metadata.author', label: 'Publisher' },
          { value: 'addedAt', label: 'Date Added' },
          { value: 'birthtimeMs', label: 'Date Created' },
          { value: 'mtimeMs', label: 'Date Modified' },
          { value: 'progress', label: 'Progress' },
          { value: 'media.numTracks', label: 'Episodes' },
          { value: 'size', label: 'Size' },
          { value: 'random', label: 'Random' }
        ];
      } else {
        options = [
          { value: 'media.metadata.title', label: 'Title' },
          { value: 'media.metadata.authorName', label: 'Author' },
          { value: 'media.metadata.authorNameLF', label: 'Author (Last, First)' },
          { value: 'media.metadata.publishedYear', label: 'Year' },
          { value: 'addedAt', label: 'Date Added' },
          { value: 'birthtimeMs', label: 'Date Created' },
          { value: 'mtimeMs', label: 'Date Modified' },
          { value: 'sequence', label: 'Sequence' },
          { value: 'progress', label: 'Progress' },
          { value: 'media.duration', label: 'Duration' },
          { value: 'size', label: 'Size' },
          { value: 'random', label: 'Random' }
        ];
      }

      sortMenu.innerHTML = options.map(opt => {
        const isSelected = currentSort === opt.value;
        return `
          <button class="sort-option-btn w-full text-left px-3 py-2 text-xs text-black-50 hover:bg-black-400 hover:text-white flex items-center justify-between transition-colors focus:outline-none ${isSelected ? 'text-accent font-medium' : ''}" data-value="${opt.value}">
            <span>${opt.label}</span>
            <span class="material-symbols text-[12px] check-icon ${isSelected ? '' : 'hidden'}">check</span>
          </button>
        `;
      }).join('');

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

      updateSortLabel(currentSort);
    };

    window.updateCustomSortMenu = renderSortMenu;
    renderSortMenu();
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
      if (showControls && (relPath !== '/library' || newStyle !== 'list')) {
        shelfSizeCtrl.classList.remove('hidden');
      } else {
        shelfSizeCtrl.classList.add('hidden');
      }
    }

    const activeLibId = getActiveLibraryId();
    if (activeLibId) {
      let pathName = window.location.pathname;
      if (typeof ROUTER_BASE_PATH !== 'undefined' && ROUTER_BASE_PATH && pathName.startsWith(ROUTER_BASE_PATH)) {
        pathName = pathName.substring(ROUTER_BASE_PATH.length);
      }
      if (!pathName.startsWith('/')) {
        pathName = '/' + pathName;
      }
      loadDashboard(activeLibId, pathName === '/');
    }
  };

  if (styleBtnShelf) styleBtnShelf.onclick = () => setStyle('shelf');
  if (styleBtnGrid) styleBtnGrid.onclick = () => setStyle('grid');
  if (styleBtnList) styleBtnList.onclick = () => setStyle('list');

  // Mobile / Desktop Menu Drawer Toggle
  const mobileMenuBtn = document.getElementById('mobile-menu-btn');
  const sidebar = document.getElementById('sidebar');
  const backdrop = document.getElementById('sidebar-backdrop');
  if (mobileMenuBtn && sidebar) {
    // Initialize desktop sidebar collapse state from localStorage
    const initSidebarCollapse = () => {
      const isCollapsed = localStorage.getItem('sidebar-collapsed') === 'true';
      if (isCollapsed) {
        sidebar.classList.add('collapsed');
      } else {
        sidebar.classList.remove('collapsed');
      }
    };
    initSidebarCollapse();

    const openMobileSidebar = () => {
      sidebar.classList.remove('hidden');
      // Trigger layout reflow for CSS transition
      sidebar.offsetHeight;
      sidebar.classList.add('open');
      if (backdrop) {
        backdrop.classList.remove('hidden');
        backdrop.offsetHeight;
        backdrop.classList.add('show');
      }
    };

    const closeMobileSidebar = () => {
      sidebar.classList.remove('open');
      if (backdrop) {
        backdrop.classList.remove('show');
      }
      setTimeout(() => {
        if (!sidebar.classList.contains('open')) {
          sidebar.classList.add('hidden');
          sidebar.classList.remove('fixed', 'left-0', 'top-16', 'bottom-0', 'flex');
        }
        if (backdrop && !backdrop.classList.contains('show')) {
          backdrop.classList.add('hidden');
        }
      }, 200);
    };

    // Attach global helper so click handlers can close sidebar easily
    window.closeMobileSidebar = closeMobileSidebar;

    mobileMenuBtn.onclick = (e) => {
      e.stopPropagation();
      if (window.innerWidth < 768) {
        const isOpen = sidebar.classList.contains('open');
        if (isOpen) {
          closeMobileSidebar();
        } else {
          openMobileSidebar();
        }
      } else {
        sidebar.classList.toggle('collapsed');
        const isCollapsedNow = sidebar.classList.contains('collapsed');
        localStorage.setItem('sidebar-collapsed', isCollapsedNow ? 'true' : 'false');
      }
    };

    // Close when clicking anywhere else
    document.addEventListener('click', (e) => {
      if (sidebar.classList.contains('open') && !sidebar.contains(e.target) && e.target !== mobileMenuBtn) {
        closeMobileSidebar();
      }
    });

    if (backdrop) {
      backdrop.onclick = (e) => {
        e.stopPropagation();
        closeMobileSidebar();
      };
    }

    // Close when resizing window to desktop
    window.addEventListener('resize', () => {
      if (window.innerWidth >= 768) {
        sidebar.classList.remove('open');
        sidebar.classList.add('hidden');
        if (backdrop) {
          backdrop.classList.remove('show');
          backdrop.classList.add('hidden');
        }
      }
    });
  }
}

function bootstrapApp(payload) {
  // Populate Sidebar Version & Source
  const sidebarVersion = document.getElementById('sidebar-version');
  const sidebarSource = document.getElementById('sidebar-source');
  if (sidebarVersion && window.serverStatus) {
    sidebarVersion.textContent = window.serverStatus.serverVersion || 'v2.35.1';
  }
  if (sidebarSource && payload) {
    sidebarSource.textContent = payload.Source || 'debian';
  }

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

  // Initialize tasks notification/activity widget
  initNotificationWidget(user);

  // Setup Admin / Root only features
  if (user.type === 'root' || user.type === 'admin') {
    const settingsBtn = document.getElementById('user-menu-settings-btn');
    const adminBtn = document.getElementById('user-menu-admin-btn');
    if (settingsBtn) {
      settingsBtn.classList.remove('hidden');
      settingsBtn.onclick = (e) => {
        e.preventDefault();
        window.closeAllDropdowns();
        navigateTo('/settings');
      };
    }
    if (adminBtn) {
      adminBtn.classList.remove('hidden');
      adminBtn.onclick = (e) => {
        e.preventDefault();
        window.closeAllDropdowns();
        navigateTo('/settings#users');
      };
    }

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

  }

  const canUpload = user.type === 'root' || user.type === 'admin' || (user.permissions && user.permissions.upload);
  const headerUploadBtn = document.getElementById('header-upload-btn');
  if (headerUploadBtn) {
    if (canUpload) {
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
    } else {
      headerUploadBtn.classList.add('hidden');
    }
  }

  const uploadBtn = document.getElementById('upload-btn');
  if (uploadBtn) {
    if (canUpload) {
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
    } else {
      uploadBtn.classList.add('hidden');
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
        item.classList.remove('bg-black-300');
      });
    };

    const highlightSearchResult = (items, index) => {
      clearSearchResultHighlight(items);
      if (index >= 0 && index < items.length) {
        const item = items[index];
        item.classList.add('bg-black-300');
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
        headerLi.className = 'px-3 py-1.5 text-[10px] font-bold text-accent uppercase tracking-wider select-none';
        headerLi.textContent = `${title} (${items.length})`;
        globalSearchResultsList.appendChild(headerLi);

        items.forEach(item => {
          const li = document.createElement('li');
          li.className = 'px-3 py-2 cursor-pointer flex items-center text-black-50';
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
            ${subtitle ? `<p class="truncate text-xs text-black-100">${escapeHtml(subtitle)}</p>` : ''}
            <p class="truncate text-xs text-black-200">${escapeHtml(authorName)}</p>
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
            <p class="truncate text-xs text-black-200">${escapeHtml(authorName)}</p>
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
            <p class="truncate text-xs text-black-200">${escapeHtml(podcastTitle)}</p>
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
            <p class="truncate text-xs text-black-200">${auth.numBooks} ${auth.numBooks === 1 ? 'Book' : 'Books'}</p>
          </div>
        `;
      });

      // 5. Series
      appendSection('Series', data.series, (li, ser) => {
        li.setAttribute('data-type', 'series');
        li.setAttribute('data-id', ser.id);
        li.innerHTML = `
          <div class="w-8 h-8 flex items-center justify-center mr-3 bg-black-600 rounded-sm select-none">
            <span class="material-symbols text-xl text-black-100">layers</span>
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
            <span class="material-symbols text-xl text-black-100">record_voice_over</span>
          </div>
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(narr.name)}</p>
            <p class="truncate text-xs text-black-200">${narr.numBooks} ${narr.numBooks === 1 ? 'Book' : 'Books'}</p>
          </div>
        `;
      });

      // 7. Tags
      appendSection('Tags', data.tags, (li, tag) => {
        li.setAttribute('data-type', 'tag');
        li.setAttribute('data-val', tag.name);
        li.innerHTML = `
          <div class="w-8 h-8 flex items-center justify-center mr-3 select-none">
            <span class="material-symbols text-xl text-black-100">local_offer</span>
          </div>
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(tag.name)}</p>
            <p class="truncate text-xs text-black-200">${tag.numItems} ${tag.numItems === 1 ? 'Item' : 'Items'}</p>
          </div>
        `;
      });

      // 8. Genres
      appendSection('Genres', data.genres, (li, gen) => {
        li.setAttribute('data-type', 'genre');
        li.setAttribute('data-val', gen.name);
        li.innerHTML = `
          <div class="w-8 h-8 flex items-center justify-center mr-3 select-none">
            <span class="material-symbols text-xl text-black-100">category</span>
          </div>
          <div class="grow min-w-0">
            <p class="truncate text-sm font-medium text-white">${escapeHtml(gen.name)}</p>
            <p class="truncate text-xs text-black-200">${gen.numItems} ${gen.numItems === 1 ? 'Item' : 'Items'}</p>
          </div>
        `;
      });

      if (totalResults === 0) {
        globalSearchResultsList.innerHTML = `<li class="text-center py-4 text-black-100 select-none">No results found</li>`;
      }

      const announcementEl = document.getElementById('global-search-announcement');
      if (announcementEl) {
        if (totalResults === 0) {
          announcementEl.textContent = 'No search results found';
        } else {
          announcementEl.textContent = `Found ${totalResults} search results for ${query}`;
        }
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
            navigateTo('/library');
          } else if (type === 'genre') {
            const val = el.getAttribute('data-val');
            const encoded = btoa(unescape(encodeURIComponent(val)));
            localStorage.setItem('library-filterBy', 'genres.' + encoded);
            const labelEl = document.getElementById('filter-selected-label');
            if (labelEl) labelEl.textContent = 'Genre: ' + val;
            navigateTo('/library');
          } else if (type === 'narrator') {
            const val = el.getAttribute('data-val');
            const encoded = btoa(unescape(encodeURIComponent(val)));
            localStorage.setItem('library-filterBy', 'narrators.' + encoded);
            const labelEl = document.getElementById('filter-selected-label');
            if (labelEl) labelEl.textContent = 'Narrator: ' + val;
            navigateTo('/library');
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
        globalSearchResultsList.innerHTML = `<li class="text-center py-4 text-black-100 select-none"><span class="material-symbols animate-spin">sync</span></li>`;
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
            const announcementEl = document.getElementById('global-search-announcement');
            if (announcementEl) {
              announcementEl.textContent = 'Error loading search results';
            }
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
        if (relPath === '/' && query) {
          navigateTo('/library', true);
          return;
        }
        const isDashboard = (relPath === '/' || relPath === '/library');
        if (isDashboard) {
          const isHome = (relPath === '/');
          loadDashboard(activeLibId, isHome);
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
            navigateTo('/library');
          } else if (activeLibId) {
            const isHome = (relPath === '/');
            loadDashboard(activeLibId, isHome);
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
  updateSearchPlaceholder();

  // Transition to main view
  showAppContainer();

  // Connect websocket and listen to events
  const token = localStorage.getItem('token');
  if (token) {
    connectSocket(token);
    registerAppSocketListeners();
  }

  // Update sidebar item visibility on initial load
  updateSidebarVisibility();

  // Route to the current URL path on bootstrap
  navigateTo(window.location.pathname, false);
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
  const canUpload = user && (user.type === 'root' || user.type === 'admin' || (user.permissions && user.permissions.upload));
  if (!token || !canUpload) return;

  e.preventDefault();
  showDragOverlay();
});

window.addEventListener('dragleave', (e) => {
  if (e.clientX <= 0 || e.clientY <= 0 || e.clientX >= window.innerWidth || e.clientY >= window.innerHeight) {
    hideDragOverlay();
  }
});

window.addEventListener('dragend', () => {
  hideDragOverlay();
});

async function readAllDirectoryEntries(dirReader) {
  let allEntries = [];
  while (true) {
    const entries = await new Promise((resolve, reject) => {
      dirReader.readEntries(resolve, reject);
    });
    if (entries.length === 0) break;
    allEntries.push(...entries);
  }
  return allEntries;
}

async function getFilesFromEntry(entry, path = '') {
  if (entry.isFile) {
    const file = await new Promise((resolve, reject) => entry.file(resolve, reject));
    return [{ file, path: path + file.name }];
  } else if (entry.isDirectory) {
    const dirReader = entry.createReader();
    const entries = await readAllDirectoryEntries(dirReader);
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
  const canUpload = user && (user.type === 'root' || user.type === 'admin' || (user.permissions && user.permissions.upload));
  if (!token || !canUpload) return;

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

function initNotificationWidget(user) {
  const widget = document.getElementById('header-notification-widget');
  const btn = document.getElementById('header-notification-btn');
  const dropdown = document.getElementById('header-notification-dropdown');
  const list = document.getElementById('header-notification-list');
  const icon = document.getElementById('header-notification-icon');
  const badge = document.getElementById('header-notification-badge');
  const badgePing = document.getElementById('header-notification-badge-ping');

  if (!widget || !btn || !dropdown || !list || !icon || !badge || !badgePing) return;

  const isAdminOrUp = user.type === 'root' || user.type === 'admin';
  if (!isAdminOrUp) {
    widget.classList.add('hidden');
    return;
  }

  let isOpen = false;
  let pollInterval = null;
  const tasksSeen = new Set();

  dropdown.closeDropdown = () => {
    if (!isOpen) return;
    isOpen = false;
    dropdown.classList.add('hidden');
  };

  btn.onclick = (e) => {
    e.stopPropagation();
    window.closeAllDropdowns(dropdown);
    if (isOpen) {
      dropdown.closeDropdown();
    } else {
      isOpen = true;
      dropdown.classList.remove('hidden');
      
      // Mark all current tasks as seen when opening menu
      const items = list.querySelectorAll('[data-task-id]');
      items.forEach(el => {
        const tid = el.getAttribute('data-task-id');
        if (tid) tasksSeen.add(tid);
      });
      badge.classList.add('hidden');
      badgePing.classList.add('hidden');
      
      fetchTasks();
    }
  };

  dropdown.onclick = (e) => {
    e.stopPropagation();
  };

  async function fetchTasks() {
    try {
      const data = await request('GET', '/api/tasks');
      const tasks = data.tasks || [];
      renderTasks(tasks);
    } catch (err) {
      console.warn('Failed to fetch tasks:', err);
    }
  }

  // Escape HTML helper
  const escapeHtml = (str) => {
    if (!str) return '';
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  };

  function renderTasks(tasks) {
    if (tasks.length === 0) {
      widget.classList.add('hidden');
      dropdown.classList.add('hidden');
      isOpen = false;
      return;
    }

    widget.classList.remove('hidden');

    // Check if any tasks are running (status is pending or downloading)
    const hasRunningTasks = tasks.some(t => t.status === 'downloading' || t.status === 'pending');
    if (hasRunningTasks) {
      icon.textContent = 'sync';
      icon.classList.add('animate-spin');
    } else {
      icon.textContent = 'notifications';
      icon.classList.remove('animate-spin');
    }

    // Unseen success indicator for finished/failed tasks that user has not clicked to see yet
    if (!isOpen) {
      const hasUnseenFinished = tasks.some(t => 
        (t.status === 'finished' || t.status === 'failed') && !tasksSeen.has(t.id)
      );
      if (hasUnseenFinished) {
        badge.classList.remove('hidden');
        badgePing.classList.remove('hidden');
      } else {
        badge.classList.add('hidden');
        badgePing.classList.add('hidden');
      }
    }

    list.innerHTML = '';
    tasks.forEach(task => {
      // Add to seen if menu is open
      if (isOpen) {
        tasksSeen.add(task.id);
      }

      const li = document.createElement('li');
      li.setAttribute('data-task-id', task.id);
      li.className = 'px-3 py-2.5 border-b border-black-400 last:border-b-0 flex items-center justify-between text-xs text-white bg-black-500/30 rounded';

      const isTaskFinished = task.status === 'finished';
      const isTaskFailed = task.status === 'failed';
      const isTaskActive = task.status === 'downloading' || task.status === 'pending';

      let statusIconHtml = '';
      if (isTaskActive) {
        statusIconHtml = `<span class="material-symbols text-base animate-spin text-accent mr-2.5 mt-0.5">sync</span>`;
      } else if (isTaskFinished) {
        statusIconHtml = `<span class="material-symbols text-base text-green-500 mr-2.5 mt-0.5">done</span>`;
      } else if (isTaskFailed) {
        statusIconHtml = `<span class="material-symbols text-base text-red-500 mr-2.5 mt-0.5">error</span>`;
      } else {
        statusIconHtml = `<span class="material-symbols text-base text-gray-400 mr-2.5 mt-0.5">cloud_download</span>`;
      }

      // Format progress and speed/bytes info
      let progressHtml = '';
      if (isTaskActive && typeof task.progress === 'number') {
        progressHtml = `
          <div class="w-full bg-black/40 h-1.5 rounded-full mt-2 overflow-hidden">
            <div class="bg-accent h-full transition-all duration-300" style="width: ${task.progress}%"></div>
          </div>
          <div class="flex justify-between text-[10px] text-gray-400 mt-1">
            <span>${task.progress}%</span>
            <span>${task.speed || ''}</span>
          </div>
        `;
      }

      const description = task.description || `Downloading: ${task.episodeTitle || 'episode'}`;
      const failedMsg = isTaskFailed && task.error ? `<p class="text-red-400 text-[10px] mt-1 font-mono">${escapeHtml(task.error)}</p>` : '';

      li.innerHTML = `
        <div class="flex items-start grow min-w-0 mr-3">
          ${statusIconHtml}
          <div class="grow min-w-0">
            <p class="font-semibold truncate text-white text-[12px]">${escapeHtml(task.episodeTitle || 'Podcast Episode')}</p>
            <p class="text-gray-300 truncate text-[11px] mt-0.5">${escapeHtml(task.podcastTitle || description)}</p>
            ${failedMsg}
            ${progressHtml}
          </div>
        </div>
        ${(isTaskActive) ? `
          <button class="bg-black-400 hover:bg-red-900/40 border border-red-500/30 text-error hover:text-white hover:border-red-500/50 px-2 py-1 rounded text-[10px] transition-colors flex-shrink-0 cursor-pointer" data-cancel-id="${task.id}">
            Cancel
          </button>
        ` : ''}
      `;

      // Wire cancel button
      const cancelBtn = li.querySelector(`[data-cancel-id="${task.id}"]`);
      if (cancelBtn) {
        cancelBtn.onclick = async (e) => {
          e.stopPropagation();
          cancelBtn.disabled = true;
          cancelBtn.textContent = 'Canceling...';
          try {
            await request('POST', `/api/tasks/${task.id}/cancel`);
            showToast('Task cancel requested', 'success');
            fetchTasks();
          } catch (err) {
            showToast('Failed to cancel task: ' + err.message, 'error');
            cancelBtn.disabled = false;
            cancelBtn.textContent = 'Cancel';
          }
        };
      }

      list.appendChild(li);
    });
  }

  // Start polling
  fetchTasks();
  pollInterval = setInterval(fetchTasks, 10000);

  // Clean up on user logout or unload
  window.addEventListener('beforeunload', () => {
    if (pollInterval) clearInterval(pollInterval);
  });
}

// Global Stack and MutationObserver for Modal Focus Trapping (Keyboard Accessibility)
let focusStack = [];
const focusableSelectors = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

function setupFocusTrap(modal) {
  const previouslyFocused = document.activeElement;
  focusStack.push({ modal, previouslyFocused });

  const getFocusables = () => {
    return Array.from(modal.querySelectorAll(focusableSelectors))
      .filter(el => {
        if (el.hasAttribute('disabled') || el.disabled) return false;
        const style = window.getComputedStyle(el);
        if (style.display === 'none' || style.visibility === 'hidden') return false;
        if (el.offsetParent === null && style.position !== 'fixed') return false;
        return true;
      });
  };

  // Focus the first appropriate element
  setTimeout(() => {
    const focusables = getFocusables();
    if (focusables.length > 0) {
      const input = focusables.find(el => el.tagName === 'INPUT' && el.type !== 'hidden' && el.type !== 'submit');
      if (input) {
        input.focus();
      } else {
        focusables[0].focus();
      }
    }
  }, 100);

  const handleKeydown = (e) => {
    if (e.key === 'Escape') {
      const top = focusStack[focusStack.length - 1];
      if (!top || top.modal !== modal) return;
      e.preventDefault();
      // Try to find a cancel/close button and trigger it, otherwise fallback to remove
      const closeBtn = modal.querySelector('button[id*="close"], button[id*="cancel"], button[class*="close"], button[class*="cancel"], [data-dismiss="modal"]');
      if (closeBtn) {
        closeBtn.click();
      } else {
        modal.remove();
      }
      return;
    }

    if (e.key !== 'Tab') return;
    
    const top = focusStack[focusStack.length - 1];
    if (!top || top.modal !== modal) return;

    const focusables = getFocusables();
    if (focusables.length === 0) {
      e.preventDefault();
      return;
    }

    const first = focusables[0];
    const last = focusables[focusables.length - 1];

    if (e.shiftKey) { // Shift + Tab
      if (document.activeElement === first) {
        last.focus();
        e.preventDefault();
      }
    } else { // Tab
      if (document.activeElement === last) {
        first.focus();
        e.preventDefault();
      }
    }
  };

  modal.addEventListener('keydown', handleKeydown);
}

function teardownFocusTrap(modal) {
  const index = focusStack.findIndex(item => item.modal === modal);
  if (index !== -1) {
    const { previouslyFocused } = focusStack[index];
    focusStack.splice(index, 1);
    if (previouslyFocused && typeof previouslyFocused.focus === 'function') {
      setTimeout(() => {
        previouslyFocused.focus();
      }, 50);
    }
  }
}

const modalObserver = new MutationObserver((mutations) => {
  for (const mutation of mutations) {
    for (const node of mutation.addedNodes) {
      if (node.nodeType === Node.ELEMENT_NODE && node.classList.contains('fixed') && (node.classList.contains('inset-0') || (node.classList.contains('w-full') && node.classList.contains('h-full')))) {
        if (node.classList.contains('pointer-events-none') || node.id === 'toast-container') continue;
        setupFocusTrap(node);
      }
    }
    for (const node of mutation.removedNodes) {
      if (node.nodeType === Node.ELEMENT_NODE && node.classList.contains('fixed') && (node.classList.contains('inset-0') || (node.classList.contains('w-full') && node.classList.contains('h-full')))) {
        if (node.classList.contains('pointer-events-none') || node.id === 'toast-container') continue;
        teardownFocusTrap(node);
      }
    }
  }
});
modalObserver.observe(document.body, { childList: true });


