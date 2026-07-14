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

      const card = document.getElementById('public-share-card');
      const coverContainer = coverImg.parentElement;
      if (response.embeddable) {
        card.className = 'w-full max-w-sm bg-primary rounded-lg shadow-lg border border-black-600/50 p-4 flex flex-col items-center space-y-4';
        if (coverContainer) {
          coverContainer.className = 'w-24 h-36 flex-shrink-0 bg-black-700 rounded-md overflow-hidden shadow-md border border-black-600 flex items-center justify-center relative';
        }
        titleEl.className = 'text-xl font-bold tracking-wide mb-0.5 text-center truncate w-full';
        authorEl.className = 'text-xs text-accent font-medium mb-2 text-center truncate w-full';
        descEl.classList.add('hidden');
        narratorDiv.classList.add('hidden');
        downloadBtn.className = 'w-full bg-accent hover:opacity-90 text-primary font-bold py-1.5 rounded flex items-center justify-center space-x-1.5 transition duration-150 text-xs';
      } else {
        card.className = 'w-full max-w-2xl bg-primary rounded-lg shadow-2xl border border-black-600/50 p-8 flex flex-col md:flex-row items-center md:items-start space-y-6 md:space-y-0 md:space-x-8';
        if (coverContainer) {
          coverContainer.className = 'w-48 h-72 flex-shrink-0 bg-black-700 rounded-md overflow-hidden shadow-md border border-black-600 flex items-center justify-center relative';
        }
        titleEl.className = 'text-3xl font-bold tracking-wide mb-1 text-center md:text-left';
        authorEl.className = 'text-lg text-accent font-medium mb-4 text-center md:text-left';
        descEl.className = 'text-sm text-black-100 line-clamp-6 mb-6 leading-relaxed text-center md:text-left';
        downloadBtn.className = 'w-full bg-accent hover:opacity-90 text-primary font-bold py-3 rounded flex items-center justify-center space-x-2 transition duration-150';
      }

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
      if (!response.embeddable && media.narrators && media.narrators.length > 0) {
        narratorEl.textContent = media.narrators.join(', ');
        narratorDiv.classList.remove('hidden');
      } else {
        narratorDiv.classList.add('hidden');
      }

      // Description
      if (!response.embeddable) {
        descEl.textContent = media.description || 'No description available.';
      }

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

      // Setup audio streaming / playlist
      let tracks = [];
      if (item.mediaType === 'book' && media.audioFiles) {
        tracks = media.audioFiles.map((af, idx) => {
          const filename = af.metadata?.filename || `Track ${idx + 1}`;
          const title = af.metaTags?.tagTitle || filename;
          return {
            index: idx,
            filename: filename,
            title: title,
            duration: af.duration || 0
          };
        });
      } else if (item.mediaType === 'podcast' && media.episodes) {
        tracks = media.episodes.map((ep, idx) => {
          const filename = ep.audioFile?.metadata?.filename || '';
          const title = ep.title || filename || `Episode ${idx + 1}`;
          return {
            index: idx,
            filename: filename,
            title: title,
            duration: ep.duration || ep.audioFile?.duration || 0
          };
        });
      }

      const tracklistContainer = document.getElementById('public-share-tracklist-container');
      const tracklistEl = document.getElementById('public-share-tracklist');
      const playerTitleEl = document.getElementById('public-share-player-title');
      
      let currentTrackIndex = 0;

      function formatDuration(seconds) {
        if (!seconds || isNaN(seconds)) return '00:00';
        const hrs = Math.floor(seconds / 3600);
        const mins = Math.floor((seconds % 3600) / 60);
        const secs = Math.floor(seconds % 60);
        if (hrs > 0) {
          return `${hrs}:${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
        }
        return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
      }

      function playTrack(idx, shouldPlay = true) {
        if (idx < 0 || idx >= tracks.length) return;
        currentTrackIndex = idx;
        const track = tracks[idx];
        
        // Highlight active track
        if (tracklistEl) {
          const trackButtons = tracklistEl.querySelectorAll('button');
          trackButtons.forEach((btn, i) => {
            if (i === idx) {
              btn.classList.add('bg-black-500', 'text-accent');
              btn.classList.remove('text-black-100', 'hover:bg-black-500', 'hover:text-white');
            } else {
              btn.classList.remove('bg-black-500', 'text-accent');
              btn.classList.add('text-black-100', 'hover:bg-black-500', 'hover:text-white');
            }
          });
        }
        
        // Update Title
        if (playerTitleEl) {
          playerTitleEl.textContent = `Now Playing: ${track.title}`;
        }
        
        // Load stream source
        const passParam = password ? `&password=${encodeURIComponent(password)}` : '';
        const trackParam = track.filename ? `&track=${encodeURIComponent(track.filename)}` : '';
        audioEl.src = resolvePath(`/api/s/${slug}/stream?raw=1${passParam}${trackParam}`);
        
        if (shouldPlay) {
          audioEl.play().catch(err => {
            console.log("Auto-play prevented or failed:", err);
          });
        }
      }

      if (tracks.length > 0) {
        audioPlayerContainer.classList.remove('hidden');
        
        // Render tracklist if multiple tracks
        if (tracks.length > 1 && tracklistContainer && tracklistEl) {
          tracklistContainer.classList.remove('hidden');
          tracklistEl.innerHTML = '';
          tracks.forEach((track, idx) => {
            const trackItem = document.createElement('button');
            trackItem.className = 'w-full text-left px-3 py-2 rounded flex justify-between items-center transition duration-150 text-black-100 hover:bg-black-500 hover:text-white focus:outline-none';
            trackItem.dataset.index = idx;
            
            const titleSpan = document.createElement('span');
            titleSpan.className = 'truncate pr-2 font-medium';
            titleSpan.textContent = `${idx + 1}. ${track.title}`;
            
            const durSpan = document.createElement('span');
            durSpan.className = 'text-xs text-black-300 flex-shrink-0';
            durSpan.textContent = formatDuration(track.duration);
            
            trackItem.appendChild(titleSpan);
            trackItem.appendChild(durSpan);
            
            trackItem.onclick = () => {
              playTrack(idx, true);
            };
            
            tracklistEl.appendChild(trackItem);
          });
        } else if (tracklistContainer) {
          tracklistContainer.classList.add('hidden');
        }

        // Initialize first track but do not force autoplay immediately on page load
        playTrack(0, false);

        // Auto-advance track on end
        audioEl.onended = () => {
          if (currentTrackIndex + 1 < tracks.length) {
            playTrack(currentTrackIndex + 1, true);
          }
        };
      } else if (item.mediaType === 'book' || item.mediaType === 'podcast') {
        // Fallback if tracks array is empty but metadata specifies it is streamable
        audioEl.src = resolvePath(`/api/s/${slug}/stream${query ? query : ''}`);
        audioPlayerContainer.classList.remove('hidden');
        if (tracklistContainer) tracklistContainer.classList.add('hidden');
        if (playerTitleEl) playerTitleEl.textContent = 'Streaming Preview';
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
