// js/app.js

import { initAuth, showAppContainer, logout } from './auth.js';
import { initLibrary, getActiveLibraryId } from './library.js';
import { loadDashboard } from './dashboard.js';
import { request } from './api.js';
import { connectSocket, disconnectSocket, onEvent } from './socket.js';
import { loadSettings } from './settings.js';
import { loadPlaylists } from './playlists.js';
import { loadCollections } from './collections.js';
import { loadAuthors, loadSeries } from './authors.js';

document.addEventListener('DOMContentLoaded', () => {
  setupEventHandlers();
  
  // Initialize Auth on page load
  initAuth().then(payload => {
    if (payload) {
      bootstrapApp(payload);
    }
  });
});

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
      sidebarLinks.forEach(l => {
        l.classList.remove('bg-primary/80', 'text-accent');
        l.classList.add('hover:bg-black-500');
        const activeBar = l.querySelector('.active-indicator');
        if (activeBar) activeBar.classList.add('hidden');
      });

      link.classList.remove('hover:bg-black-500');
      link.classList.add('bg-primary/80', 'text-accent');
      const activeBar = link.querySelector('.active-indicator');
      if (activeBar) activeBar.classList.remove('hidden');

      const pageName = link.querySelector('p').textContent;
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
    const activeLink = document.querySelector('#siderail-buttons-container a.bg-primary/80');
    const pageName = activeLink ? activeLink.querySelector('p').textContent : 'Home';
    if (pageName === 'Playlists') {
      loadPlaylists(libraryId);
    } else if (pageName === 'Collections') {
      loadCollections(libraryId);
    } else if (pageName === 'Authors') {
      loadAuthors(libraryId);
    } else if (pageName === 'Series') {
      loadSeries(libraryId);
    } else {
      loadDashboard(libraryId);
    }
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
  // Populate User Identity Details
  const user = payload.user || {};
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

function showToast(message, type = 'info') {
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
  const activeLink = document.querySelector('#siderail-buttons-container a.bg-primary/80');
  if (!activeLink) return false;
  const pageName = activeLink.querySelector('p').textContent.trim();
  const hasDetailsBtn = !!document.getElementById('details-back-btn');
  return (pageName === 'Home' || pageName === 'Library') && !hasDetailsBtn;
}
