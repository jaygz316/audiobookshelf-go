// js/app.js

import { initAuth, showAppContainer, logout } from './auth.js';
import { initLibrary, getActiveLibraryId } from './library.js';
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
      link.classList.remove('hover:bg-black-500');
      link.classList.add('bg-primary/80', 'text-accent');
      const activeBar = link.querySelector('.active-indicator');
      if (activeBar) activeBar.classList.remove('hidden');
    } else {
      link.classList.remove('bg-primary/80', 'text-accent');
      link.classList.add('hover:bg-black-500');
      const activeBar = link.querySelector('.active-indicator');
      if (activeBar) activeBar.classList.add('hidden');
    }
  });
}

function setupEventHandlers() {
  // Credentials Form Submission
  const loginForm = document.getElementById('login-form');
  if (loginForm) {
    loginForm.onsubmit = async (e) => {
      e.preventDefault();
      const usernameInput = document.getElementById('username');
      const passwordInput = document.getElementById('password');
      const loginError = document.getElementById('login-error');

      loginError.classList.add('hidden');
      loginError.textContent = '';

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
      
      const pageName = link.querySelector('p').textContent.trim();
      let path = '/';
      if (pageName === 'Playlists') path = '/playlists';
      else if (pageName === 'Collections') path = '/collections';
      else if (pageName === 'Authors') path = '/authors';
      else if (pageName === 'Series') path = '/series';
      else if (pageName === 'Narrators') path = '/narrators';
      else if (pageName === 'Stats') path = '/stats';

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

    highlightSidebarLink('Home');

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

    // Update Filter UI elements
    const filterLabels = {
      "": "Filter: All",
      "progress.not-started": "Unstarted",
      "progress.in-progress": "In Progress",
      "progress.finished": "Completed"
    };

    const updateFilterLabel = (val) => {
      const labelEl = document.getElementById('filter-selected-label');
      if (labelEl) labelEl.textContent = filterLabels[val] || 'Filter: All';
      
      filterMenu.querySelectorAll('.filter-option-btn').forEach(btn => {
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

    updateFilterLabel(activeFilter);

    filterMenu.querySelectorAll('.filter-option-btn').forEach(btn => {
      btn.onclick = (e) => {
        e.stopPropagation();
        const val = btn.getAttribute('data-value');
        localStorage.setItem('library-filterBy', val);
        updateFilterLabel(val);
        closeFilter();
        const activeLibId = getActiveLibraryId();
        if (activeLibId) loadDashboard(activeLibId);
      };
    });

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
      const showControls = (relPath === '/' || relPath === '/library');
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

  // Route to the current URL path on bootstrap
  navigateTo(window.location.pathname, false);
}

function navigateTo(path, pushState = true) {
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
    if (showControls && currentStyle !== 'list') shelfSizeControl.classList.remove('hidden');
    else shelfSizeControl.classList.add('hidden');
  }
  if (styleSwitcher) {
    if (showControls) styleSwitcher.classList.remove('hidden');
    else styleSwitcher.classList.add('hidden');
  }

  const activeLibId = getActiveLibraryId();

  if (relPath === '/' || relPath === '/library') {
    highlightSidebarLink('Home');
    const viewTitle = document.getElementById('view-title');
    if (viewTitle) viewTitle.textContent = 'Home';
    if (activeLibId) loadDashboard(activeLibId);
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
  } else if (relPath === '/settings') {
    // Deselect sidebar highlights
    document.querySelectorAll('#siderail-buttons-container a').forEach(l => {
      l.classList.remove('bg-primary/80', 'text-accent');
      l.classList.add('hover:bg-black-500');
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


