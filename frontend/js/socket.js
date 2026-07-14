// js/socket.js

import { resolvePath } from './api.js';

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
