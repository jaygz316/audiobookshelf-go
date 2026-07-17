// js/socket.js

import { resolvePath, ROUTER_BASE_PATH } from './api.js';
import { getActiveLibraryId } from './library.js';
import { isDashboardActive } from './router.js';
import { loadDashboard, progressCache } from './dashboard.js';
import { showToast } from './toast.js';

let ws = null;
let pingIntervalId = null;
const eventListeners = {};

export function connectSocket(token) {
  if (ws) {
    disconnectSocket();
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const wsPath = resolvePath('/socket.io/');
  const wsUrl = `${protocol}//${host}${wsPath}?EIO=4&transport=websocket`;

  console.log('[Socket] Connecting to:', wsUrl);
  ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    console.log('[Socket] WebSocket open');
  };

  ws.onmessage = (event) => {
    const packet = event.data;
    if (typeof packet !== 'string') return;

    // Handle Engine.io frame types
    if (packet.startsWith('0')) {
      // Engine.io open frame -> reply with Socket.io connect frame
      ws.send('40');
    } else if (packet.startsWith('40')) {
      // Socket.io connect response -> authenticate
      ws.send(`42${JSON.stringify(["auth", token])}`);
      startHeartbeat();
    } else if (packet.startsWith('42')) {
      // Socket.io event
      try {
        const payload = JSON.parse(packet.substring(2));
        if (Array.isArray(payload) && payload.length > 0) {
          const eventName = payload[0];
          const eventData = payload[1] !== undefined ? payload[1] : null;
          triggerEvent(eventName, eventData);
        }
      } catch (err) {
        console.error('[Socket] Failed to parse Socket.io event:', err, packet);
      }
    } else if (packet === '3') {
      // Engine.io pong response to Engine.io ping
    }
  };

  ws.onclose = (e) => {
    console.log('[Socket] WebSocket closed:', e.code, e.reason);
    stopHeartbeat();
    ws = null;
  };

  ws.onerror = (err) => {
    console.error('[Socket] WebSocket error:', err);
  };
}

export function disconnectSocket() {
  stopHeartbeat();
  if (ws) {
    ws.close();
    ws = null;
  }
}

export function sendEvent(event, data = null) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(`42${JSON.stringify([event, data])}`);
  } else {
    console.warn('[Socket] Cannot send event, socket not connected');
  }
}

export function onEvent(event, callback) {
  if (!eventListeners[event]) {
    eventListeners[event] = [];
  }
  eventListeners[event].push(callback);
}

export function offEvent(event, callback) {
  if (eventListeners[event]) {
    eventListeners[event] = eventListeners[event].filter(cb => cb !== callback);
  }
}

function triggerEvent(event, data) {
  const listeners = eventListeners[event];
  if (listeners) {
    listeners.forEach(cb => {
      try {
        cb(data);
      } catch (err) {
        console.error(`[Socket] Error in listener for event ${event}:`, err);
      }
    });
  }
}

function startHeartbeat() {
  stopHeartbeat();
  pingIntervalId = setInterval(() => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send('2'); // Engine.io ping
      sendEvent('ping'); // Custom socket.io ping event
    }
  }, 20000);
}

function stopHeartbeat() {
  if (pingIntervalId) {
    clearInterval(pingIntervalId);
    pingIntervalId = null;
  }
}

export function registerAppSocketListeners() {
  if (!window.activePlaybackSessions) {
    window.activePlaybackSessions = new Map();
  }

  // Socket init event
  onEvent('init', (data) => {
    console.log('[Socket] init received:', data);
    window.activePlaybackSessions.clear();
    if (data && data.playbackSessions && Array.isArray(data.playbackSessions)) {
      data.playbackSessions.forEach(s => {
        window.activePlaybackSessions.set(s.id, s);
      });
    }
    if (data && data.usersOnline && Array.isArray(data.usersOnline)) {
      data.usersOnline.forEach(u => {
        if (u.playbackSessions && Array.isArray(u.playbackSessions)) {
          u.playbackSessions.forEach(s => {
            window.activePlaybackSessions.set(s.id, s);
          });
        }
      });
    }
    document.dispatchEvent(new CustomEvent('presence-updated'));
  });

  // Playback session state sync
  onEvent('playback_session_added', (session) => {
    console.log('[Socket] global playback_session_added:', session);
    if (session && session.id) {
      window.activePlaybackSessions.set(session.id, session);
      document.dispatchEvent(new CustomEvent('presence-updated'));
    }
  });

  onEvent('playback_session_updated', (session) => {
    console.log('[Socket] global playback_session_updated:', session);
    if (session && session.id) {
      window.activePlaybackSessions.set(session.id, session);
      document.dispatchEvent(new CustomEvent('presence-updated'));
    }
  });

  onEvent('playback_session_removed', (data) => {
    console.log('[Socket] global playback_session_removed:', data);
    if (data && data.id) {
      window.activePlaybackSessions.delete(data.id);
      document.dispatchEvent(new CustomEvent('presence-updated'));
    }
  });

  // Register listener for progress syncing across devices
  onEvent('user_item_progress_updated', (data) => {
    console.log('[Socket] progress updated:', data);
    if (!data || !data.itemId) return;

    const cacheKey = data.itemId;
    const oldProgress = progressCache.get(cacheKey);

    const newProgress = {
      progress: data.progress,
      isFinished: data.isFinished,
      currentTime: data.currentTime,
      duration: data.duration
    };

    // Update cache
    progressCache.set(cacheKey, newProgress);

    // Dispatch event for card/UI in-place updates
    document.dispatchEvent(new CustomEvent('progress-updated', {
      detail: {
        itemId: data.itemId,
        progress: newProgress
      }
    }));

    // Determine if we need to reload dashboard shelves (membership changes)
    const oldFinished = oldProgress ? oldProgress.isFinished : false;
    const oldStarted = oldProgress ? (oldProgress.progress > 0) : false;
    const newFinished = newProgress.isFinished;
    const newStarted = newProgress.progress > 0;

    const membershipChanged = (oldFinished !== newFinished) || (oldStarted !== newStarted);

    if (membershipChanged) {
      const activeLibId = getActiveLibraryId();
      if (activeLibId && isDashboardActive()) {
        let pathName = window.location.pathname;
        if (typeof ROUTER_BASE_PATH !== 'undefined' && ROUTER_BASE_PATH && pathName.startsWith(ROUTER_BASE_PATH)) {
          pathName = pathName.substring(ROUTER_BASE_PATH.length);
        }
        if (!pathName.startsWith('/')) {
          pathName = '/' + pathName;
        }
        loadDashboard(activeLibId, pathName === '/');
      }
    }
  });

  onEvent('user_updated', (data) => {
    console.log('[Socket] user updated:', data);
    const activeLibId = getActiveLibraryId();
    if (activeLibId && isDashboardActive()) {
      let pathName = window.location.pathname;
      if (typeof ROUTER_BASE_PATH !== 'undefined' && ROUTER_BASE_PATH && pathName.startsWith(ROUTER_BASE_PATH)) {
        pathName = pathName.substring(ROUTER_BASE_PATH.length);
      }
      if (!pathName.startsWith('/')) {
        pathName = '/' + pathName;
      }
      loadDashboard(activeLibId, pathName === '/');
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
        let pathName = window.location.pathname;
        if (typeof ROUTER_BASE_PATH !== 'undefined' && ROUTER_BASE_PATH && pathName.startsWith(ROUTER_BASE_PATH)) {
          pathName = pathName.substring(ROUTER_BASE_PATH.length);
        }
        if (!pathName.startsWith('/')) {
          pathName = '/' + pathName;
        }
        loadDashboard(libraryId, pathName === '/');
      }
    }
  });
}
