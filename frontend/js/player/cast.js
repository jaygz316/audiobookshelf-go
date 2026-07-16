import { resolvePath } from '../api.js';

let castContext = null;
let remotePlayer = null;
let remotePlayerController = null;

let callbacks = {
  onConnected: () => {},
  onDisconnected: () => {},
  onStateChanged: () => {},
  onTimeChanged: () => {},
  onDurationChanged: () => {}
};

export function setupCastCallbacks(objs) {
  callbacks = { ...callbacks, ...objs };
}

export function isCasting() {
  return remotePlayer && remotePlayer.isConnected;
}

export function getRemotePlayer() {
  return remotePlayer;
}

export function initializeCastApi() {
  if (!window.cast || !window.cast.framework) {
    console.warn('Cast framework not available yet');
    return;
  }
  
  castContext = window.cast.framework.CastContext.getInstance();
  castContext.setOptions({
    receiverApplicationId: chrome.cast.media.DEFAULT_MEDIA_RECEIVER_APP_ID,
    autoJoinPolicy: chrome.cast.AutoJoinPolicy.ORIGINAL_SESSION_RESTRICTION
  });

  remotePlayer = new window.cast.framework.RemotePlayer();
  remotePlayerController = new window.cast.framework.RemotePlayerController(remotePlayer);

  // Listen for connection status changes
  remotePlayerController.addEventListener(
    window.cast.framework.RemotePlayerEventType.IS_CONNECTED_CHANGED,
    onCastConnectionChanged
  );

  // Listen for media status changes (play/pause state, time, etc.)
  remotePlayerController.addEventListener(
    window.cast.framework.RemotePlayerEventType.PLAYER_STATE_CHANGED,
    onCastStateChanged
  );

  remotePlayerController.addEventListener(
    window.cast.framework.RemotePlayerEventType.CURRENT_TIME_CHANGED,
    onCastTimeChanged
  );

  remotePlayerController.addEventListener(
    window.cast.framework.RemotePlayerEventType.DURATION_CHANGED,
    onCastDurationChanged
  );
}

// Attach callback to window
window.__onGCastApiAvailable = function(isAvailable) {
  if (isAvailable) {
    initializeCastApi();
  }
};

// Check if already available (safeguard)
if (window.cast && window.cast.framework) {
  initializeCastApi();
}

function onCastConnectionChanged() {
  if (remotePlayer && remotePlayer.isConnected) {
    console.log('Chromecast connected');
    callbacks.onConnected();
  } else {
    console.log('Chromecast disconnected');
    const remoteTime = (remotePlayer && remotePlayer.currentTime) || 0;
    callbacks.onDisconnected(remoteTime);
  }
}

function onCastStateChanged() {
  if (!remotePlayer) return;
  const state = remotePlayer.playerState;
  const isPlaying = state === chrome.cast.media.PlayerState.PLAYING;
  const isFinished = state === chrome.cast.media.PlayerState.IDLE && remotePlayer.idleReason === chrome.cast.media.IdleReason.FINISHED;
  callbacks.onStateChanged(isPlaying, isFinished);
}

function onCastTimeChanged() {
  callbacks.onTimeChanged();
}

function onCastDurationChanged() {
  callbacks.onDurationChanged();
}

export function castPlayItem(item, startTime = 0, clientPlaylistUri) {
  if (!castContext) return;
  const session = castContext.getCurrentSession();
  if (!session) return;

  // Build the absolute streaming URL
  let streamUrl = window.location.origin + resolvePath(clientPlaylistUri);

  const mediaInfo = new chrome.cast.media.MediaInfo(streamUrl, 'application/vnd.apple.mpegurl');
  mediaInfo.metadata = new chrome.cast.media.GenericMediaMetadata();
  mediaInfo.metadata.metadataType = chrome.cast.media.MetadataType.MUSIC_TRACK;
  
  let title = '';
  let author = '';
  if (item.mediaType === 'book') {
    const metadata = item.media?.metadata || {};
    title = metadata.title || item.title || 'Untitled';
    author = metadata.authorName || 'Unknown';
  } else if (item.mediaType === 'podcast') {
    const metadata = item.media?.metadata || {};
    title = metadata.title || item.title || 'Untitled';
    author = metadata.author || 'Unknown';
  } else {
    title = item.title || 'Untitled';
    author = 'Unknown';
  }

  mediaInfo.metadata.title = title;
  mediaInfo.metadata.artist = author;
  
  const token = localStorage.getItem('token');
  const ts = item.updatedAt || item.addedAt || Date.now();
  const coverUrl = window.location.origin + resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);
  mediaInfo.metadata.images = [{ url: coverUrl }];

  const requestObj = new chrome.cast.media.LoadRequest(mediaInfo);
  requestObj.currentTime = startTime;

  session.loadMedia(requestObj).then(
    function() {
      console.log('Chromecast: loadMedia success');
      callbacks.onStateChanged(true, false);
    },
    function(err) {
      console.error('Chromecast: loadMedia error', err);
    }
  );
}
