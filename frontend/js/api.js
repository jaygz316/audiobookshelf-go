// js/api.js

export function getBasePath() {
  const path = window.location.pathname;
  const segments = path.split('/');
  let basePath = '';
  // If path is e.g. "/audiobookshelf/" or "/audiobookshelf/index.html", segments[1] is "audiobookshelf"
  if (segments.length > 1 && segments[1] && !segments[1].includes('.') && segments[1] !== 'index.html' && segments[1] !== 's') {
    basePath = '/' + segments[1];
  }
  return basePath;
}

export const ROUTER_BASE_PATH = getBasePath();

export function resolvePath(path) {
  if (ROUTER_BASE_PATH && !path.startsWith(ROUTER_BASE_PATH)) {
    return `${ROUTER_BASE_PATH}${path}`;
  }
  return path;
}

export async function request(method, path, body = null, options = {}) {
  const token = localStorage.getItem('token');
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers
  };

  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const config = {
    method,
    headers,
    ...options
  };

  if (body) {
    config.body = JSON.stringify(body);
  }

  const resolved = resolvePath(path);
  const response = await fetch(resolved, config);

  if (response.status === 401) {
    if (path !== '/login' && path !== '/auth/refresh') {
      window.dispatchEvent(new CustomEvent('auth-unauthorized'));
    }
    let errorText = '';
    try {
      errorText = await response.text();
    } catch (_) {}
    throw new Error(errorText || 'Unauthorized');
  }

  if (!response.ok) {
    let errorText = '';
    try {
      errorText = await response.text();
    } catch (_) {}
    throw new Error(errorText || response.statusText);
  }

  if (response.status === 204) {
    return null;
  }

  const contentType = response.headers.get('content-type');
  if (contentType && contentType.includes('application/json')) {
    return response.json();
  }
  return response.text();
}
