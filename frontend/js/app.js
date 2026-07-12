// js/app.js

import { initAuth, showAppContainer, logout } from './auth.js';
import { initLibrary, getActiveLibraryId } from './library.js';
import { loadDashboard } from './dashboard.js';
import { request } from './api.js';
import { connectSocket, disconnectSocket, onEvent } from './socket.js';
import { loadSettings, applyServerThemeAndCss } from './settings.js';
import { loadPlaylists } from './playlists.js';
import { loadCollections } from './collections.js';
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

  // User Dropdown Toggles
  const userMenuBtn = document.getElementById('user-menu-btn');
  const userDropdown = document.getElementById('user-dropdown');
  if (userMenuBtn && userDropdown) {
    userMenuBtn.onclick = (e) => {
      e.stopPropagation();
      userDropdown.classList.toggle('hidden');
    };
    document.addEventListener('click', () => {
      userDropdown.classList.add('hidden');
    });
    userDropdown.onclick = (e) => {
      e.stopPropagation();
    };
  }

  // Siderail buttons static toggles (for Milestones)
  const sidebarLinks = document.querySelectorAll('#siderail-buttons-container a');
  const viewTitle = document.getElementById('view-title');
  sidebarLinks.forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      
      const opmlBtn = document.getElementById('opml-btn');
      if (opmlBtn) opmlBtn.classList.add('hidden');

      const pageName = link.querySelector('p').textContent.trim();
      highlightSidebarLink(pageName);

      if (viewTitle) {
        viewTitle.textContent = pageName;
      }

      const activeLibId = getActiveLibraryId();
      if (pageName === 'Home' || pageName === 'Library') {
        if (activeLibId) loadDashboard(activeLibId);
      } else if (pageName === 'Playlists') {
        if (activeLibId) loadPlaylists(activeLibId);
      } else if (pageName === 'Collections') {
        if (activeLibId) loadCollections(activeLibId);
      } else if (pageName === 'Authors') {
        if (activeLibId) loadAuthors(activeLibId);
      } else if (pageName === 'Series') {
        if (activeLibId) loadSeries(activeLibId);
      } else if (pageName === 'Narrators') {
        if (activeLibId) loadNarrators(activeLibId);
      } else if (pageName === 'Stats') {
        loadStats();
      }
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
    
    // Reload the current active page
    const activeLink = document.querySelector('#siderail-buttons-container a.bg-primary\\/80');
    const pageName = activeLink ? activeLink.querySelector('p').textContent : 'Home';
    if (pageName === 'Playlists') {
      loadPlaylists(libraryId);
    } else if (pageName === 'Collections') {
      loadCollections(libraryId);
    } else if (pageName === 'Authors') {
      loadAuthors(libraryId);
    } else if (pageName === 'Series') {
      loadSeries(libraryId);
    } else if (pageName === 'Narrators') {
      loadNarrators(libraryId);
    } else {
      loadDashboard(libraryId);
    }
  });

  window.addEventListener('navigate-to-dashboard', (e) => {
    const activeLibId = getActiveLibraryId();
    if (!activeLibId) return;
    const { filterBy, filterLabel } = e.detail;

    highlightSidebarLink('Home');

    const viewTitle = document.getElementById('view-title');
    if (viewTitle) {
      viewTitle.textContent = filterLabel || 'Home';
    }

    loadDashboard(activeLibId, filterBy, filterLabel);
  });

  window.addEventListener('navigate-to-author', (e) => {
    const { authorId, authorName } = e.detail;
    highlightSidebarLink('Authors');
    const viewTitle = document.getElementById('view-title');
    if (viewTitle) {
      viewTitle.textContent = authorName || 'Author Details';
    }
    loadAuthorDetails(authorId);
  });

  window.addEventListener('navigate-to-series', (e) => {
    const { seriesId, seriesName } = e.detail;
    highlightSidebarLink('Series');
    const viewTitle = document.getElementById('view-title');
    if (viewTitle) {
      viewTitle.textContent = seriesName || 'Series Details';
    }
    loadSeriesDetails(seriesId);
  });

  // User Menu settings/admin clicks
  const settingsBtn = document.getElementById('user-menu-settings-btn');
  const adminBtn = document.getElementById('user-menu-admin-btn');
  const handleSettingsClick = (e) => {
    e.preventDefault();
    // Deselect sidebar highlights
    document.querySelectorAll('#siderail-buttons-container a').forEach(l => {
      l.classList.remove('bg-primary/80', 'text-accent');
      l.classList.add('hover:bg-black-500');
      const activeBar = l.querySelector('.active-indicator');
      if (activeBar) activeBar.classList.add('hidden');
    });
    loadSettings();
  };
  if (settingsBtn) settingsBtn.onclick = handleSettingsClick;
  if (adminBtn) adminBtn.onclick = handleSettingsClick;
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
  const userDisplayName = document.getElementById('user-display-name');
  const userDisplayRole = document.getElementById('user-display-role');
  
  if (userInitials) {
    userInitials.textContent = (user.username || 'U').substring(0, 2).toUpperCase();
  }
  if (userDisplayName) {
    userDisplayName.textContent = user.username || 'User';
  }
  if (userDisplayRole) {
    userDisplayRole.textContent = user.type || 'User';
  }

  // Setup Admin / Root only features
  if (user.type === 'root' || user.type === 'admin') {
    const settingsBtn = document.getElementById('user-menu-settings-btn');
    const adminBtn = document.getElementById('user-menu-admin-btn');
    if (settingsBtn) settingsBtn.classList.remove('hidden');
    if (adminBtn) adminBtn.classList.remove('hidden');

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

  // Load the dashboard for the active library
  const activeLibId = getActiveLibraryId();
  if (activeLibId) {
    loadDashboard(activeLibId);
  }
}

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


