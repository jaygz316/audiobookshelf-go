import { request, resolvePath } from './api.js';

export async function initPublicShare(slug) {
  // Hide standard app views & login view
  const loginScreen = document.getElementById('login-screen');
  const setupScreen = document.getElementById('setup-screen');
  const appContainer = document.getElementById('app-container');
  if (loginScreen) loginScreen.classList.add('hidden');
  if (setupScreen) setupScreen.classList.add('hidden');
  if (appContainer) appContainer.classList.add('hidden');

  // Show public share screen
  const publicShareScreen = document.getElementById('public-share-screen');
  if (publicShareScreen) {
    publicShareScreen.classList.remove('hidden');
  }

  let password = '';

  const passwordForm = document.getElementById('public-share-password-form');
  const passwordError = document.getElementById('public-share-password-error');
  const passwordInput = document.getElementById('public-share-password-input');
  const lockContainer = document.getElementById('public-share-lock-container');
  const actionsContainer = document.getElementById('public-share-actions-container');

  const coverImg = document.getElementById('public-share-cover');
  const coverPlaceholder = document.getElementById('public-share-cover-placeholder');
  const titleEl = document.getElementById('public-share-title');
  const authorEl = document.getElementById('public-share-author');
  const descEl = document.getElementById('public-share-description');
  const narratorDiv = document.getElementById('public-share-narrator-div');
  const narratorEl = document.getElementById('public-share-narrator');

  const downloadBtn = document.getElementById('public-share-download-btn');
  const audioPlayerContainer = document.getElementById('public-share-audio-player-container');
  const audioEl = document.getElementById('public-share-audio');

  async function loadShareInfo() {
    try {
      passwordError.classList.add('hidden');
      
      const query = password ? `?password=${encodeURIComponent(password)}` : '';
      const response = await request('GET', `/api/s/${slug}${query}`);
      
      // If success, render it!
      lockContainer.classList.add('hidden');
      actionsContainer.classList.remove('hidden');

      const item = response.item || {};
      const media = item.media || {};

      titleEl.textContent = media.title || item.title || 'Unknown Title';
      
      // Author name
      let authorName = 'Unknown Author';
      if (media.authors && media.authors.length > 0) {
        authorName = media.authors.map(a => a.name).join(', ');
      } else if (media.authorNamesFirstLast) {
        authorName = media.authorNamesFirstLast;
      }
      authorEl.textContent = authorName;

      // Narrator
      if (media.narrators && media.narrators.length > 0) {
        narratorEl.textContent = media.narrators.join(', ');
        narratorDiv.classList.remove('hidden');
      } else {
        narratorDiv.classList.add('hidden');
      }

      // Description
      descEl.textContent = media.description || 'No description available.';

      // Cover
      const passParam = password ? `&password=${encodeURIComponent(password)}` : '';
      coverImg.src = resolvePath(`/api/s/${slug}/cover?raw=1${passParam}`);
      coverImg.onload = () => {
        coverImg.classList.remove('hidden');
        coverPlaceholder.classList.add('hidden');
      };
      coverImg.onerror = () => {
        coverImg.classList.add('hidden');
        coverPlaceholder.classList.remove('hidden');
      };

      // Download
      if (response.isDownloadable) {
        downloadBtn.href = resolvePath(`/api/s/${slug}/download${query ? query : ''}`);
        downloadBtn.classList.remove('hidden');
      } else {
        downloadBtn.classList.add('hidden');
      }

      // Setup audio streaming if there's audio files
      const audioFiles = media.audioFiles || [];
      if (audioFiles.length > 0 || item.mediaType === 'book' || item.mediaType === 'podcast') {
        audioEl.src = resolvePath(`/api/s/${slug}/stream${query ? query : ''}`);
        audioPlayerContainer.classList.remove('hidden');
      } else {
        audioPlayerContainer.classList.add('hidden');
      }

    } catch (err) {
      console.error('Failed to load public share link info:', err);
      if (err.status === 401) {
        // Show password form
        lockContainer.classList.remove('hidden');
        actionsContainer.classList.add('hidden');
        titleEl.textContent = 'Protected Share';
        authorEl.textContent = 'Enter password to unlock';
        descEl.textContent = 'The creator of this share has restricted access with a password. Please enter it below.';
        coverImg.classList.add('hidden');
        coverPlaceholder.classList.remove('hidden');
      } else {
        titleEl.textContent = 'Share Not Found';
        authorEl.textContent = 'Link expired or invalid';
        descEl.textContent = 'The link you are trying to access is either expired, invalid, or has been removed.';
        coverImg.classList.add('hidden');
        coverPlaceholder.classList.remove('hidden');
        lockContainer.classList.add('hidden');
        actionsContainer.classList.add('hidden');
      }
    }
  }

  if (passwordForm) {
    passwordForm.onsubmit = async (e) => {
      e.preventDefault();
      password = passwordInput.value;
      await loadShareInfo();
      if (lockContainer.classList.contains('hidden')) {
        // Success
        passwordInput.value = '';
      } else {
        // Failed
        passwordError.textContent = 'Incorrect password. Please try again.';
        passwordError.classList.remove('hidden');
      }
    };
  }

  // Initial load
  await loadShareInfo();
}
