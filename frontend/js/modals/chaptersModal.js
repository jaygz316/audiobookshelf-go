import { request } from '../api.js';
import { formatDuration, escapeHtml, parseDuration } from '../itemDetails.js';

async function getWaveformPeaks(itemId) {
  try {
    const res = await request('GET', `/api/items/${itemId}/waveform`);
    if (res && Array.isArray(res.peaks) && res.peaks.length > 0) {
      return res.peaks;
    }
  } catch (e) {
    console.warn("Failed to fetch waveform peaks:", e);
  }
  
  // Fallback: Generate a high-quality mock waveform for visual feedback
  const mockPeaks = [];
  const count = 400;
  for (let i = 0; i < count; i++) {
    const angle1 = i / 6;
    const angle2 = i / 20;
    const val = Math.abs(Math.sin(angle1) * Math.cos(angle2) * 180) + Math.random() * 40 + 20;
    mockPeaks.push(Math.max(0, Math.min(255, Math.floor(val))));
  }
  return mockPeaks;
}

export function triggerEditChaptersModal(item, onSaveSuccess) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4';

  let currentChapters = JSON.parse(JSON.stringify(item.media?.chapters || []));
  if (!Array.isArray(currentChapters)) {
    currentChapters = [];
  }

  const duration = item.media?.duration || (currentChapters.length > 0 ? currentChapters[currentChapters.length - 1].end : 3600) || 3600;
  let peaks = [];
  let zoomLevel = 1.0;
  let activeDragIndex = null;

  const renderChaptersList = () => {
    const listContainer = modal.querySelector('#chapters-editor-list');
    if (!listContainer) return;

    if (currentChapters.length === 0) {
      listContainer.innerHTML = `
        <div class="text-center py-8 text-black-100 text-xs">
          No chapters. Click "Add Chapter" or "Audnexus Lookup" to populate chapters.
        </div>
      `;
      return;
    }

    listContainer.innerHTML = currentChapters.map((chap, idx) => `
      <div class="flex items-center space-x-2 bg-black-500/40 p-2 rounded border border-black-400/50 text-xs chapter-row transition-all" data-idx="${idx}">
        <span class="text-black-100 font-semibold w-6 text-center">${idx + 1}</span>
        
        <div class="flex-grow min-w-0">
          <input type="text" class="w-full bg-black-500 text-white px-2 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs chapter-title" value="${escapeHtml(chap.title)}" placeholder="Chapter Title">
        </div>
        
        <div class="w-24">
          <input type="text" class="w-full bg-black-500 text-white px-2 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs text-center chapter-start" value="${formatDuration(chap.start)}" placeholder="Start (HH:MM:SS)">
        </div>

        <div class="w-24">
          <input type="text" class="w-full bg-black-500 text-white px-2 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs text-center chapter-end" value="${formatDuration(chap.end)}" placeholder="End (HH:MM:SS)">
        </div>

        <button class="text-[10px] text-accent hover:underline px-1.5 py-1 hover:bg-black-500 rounded split-chapter-row-btn" title="Split chapter in half">
          Split
        </button>

        <button class="text-red-500 hover:text-red-400 transition-colors p-1 delete-chapter-btn" title="Delete Chapter">
          <span class="material-symbols text-sm">delete</span>
        </button>
      </div>
    `).join('');

    const rows = listContainer.querySelectorAll('.chapter-row');
    rows.forEach(row => {
      const idx = parseInt(row.getAttribute('data-idx'), 10);
      
      const titleInput = row.querySelector('.chapter-title');
      titleInput.oninput = (e) => {
        currentChapters[idx].title = e.target.value;
        const targetSeg = modal.querySelector(`#chapters-segments-container [data-idx="${idx}"] .seg-title`);
        if (targetSeg) {
          targetSeg.textContent = e.target.value;
        }
      };

      const startInput = row.querySelector('.chapter-start');
      startInput.onchange = (e) => {
        const val = parseDuration(e.target.value);
        currentChapters[idx].start = Math.max(0, Math.min(val, currentChapters[idx].end - 1));
        e.target.value = formatDuration(currentChapters[idx].start);
        
        if (idx > 0) {
          currentChapters[idx - 1].end = currentChapters[idx].start;
        }
        currentChapters.sort((a, b) => a.start - b.start);
        renderChaptersList();
        updateTimeline();
      };

      const endInput = row.querySelector('.chapter-end');
      endInput.onchange = (e) => {
        const val = parseDuration(e.target.value);
        currentChapters[idx].end = Math.max(currentChapters[idx].start + 1, Math.min(val, duration));
        e.target.value = formatDuration(currentChapters[idx].end);
        
        if (idx < currentChapters.length - 1) {
          currentChapters[idx + 1].start = currentChapters[idx].end;
        }
        currentChapters.sort((a, b) => a.start - b.start);
        renderChaptersList();
        updateTimeline();
      };

      const splitBtn = row.querySelector('.split-chapter-row-btn');
      splitBtn.onclick = (e) => {
        e.stopPropagation();
        const chap = currentChapters[idx];
        const mid = Math.round((chap.start + chap.end) / 2);
        currentChapters.splice(idx + 1, 0, {
          title: `${chap.title} (Part 2)`,
          start: mid,
          end: chap.end
        });
        chap.end = mid;
        chap.title = `${chap.title} (Part 1)`;
        renderChaptersList();
        updateTimeline();
      };

      const deleteBtn = row.querySelector('.delete-chapter-btn');
      deleteBtn.onclick = () => {
        currentChapters.splice(idx, 1);
        renderChaptersList();
        updateTimeline();
      };
      
      row.onclick = () => {
        rows.forEach(r => r.classList.remove('bg-accent/10', 'border-accent'));
        row.classList.add('bg-accent/10', 'border-accent');
        
        const segs = modal.querySelectorAll('#chapters-segments-container [data-idx]');
        segs.forEach(s => s.classList.remove('bg-accent/25', 'border-accent'));
        
        const targetSeg = modal.querySelector(`#chapters-segments-container [data-idx="${idx}"]`);
        if (targetSeg) {
          targetSeg.classList.add('bg-accent/25', 'border-accent');
          targetSeg.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'center' });
        }
      };
    });
  };

  const renderRuler = () => {
    const ruler = modal.querySelector('#timeline-ruler');
    if (!ruler) return;
    ruler.innerHTML = '';

    const containerWidth = ruler.clientWidth || ruler.offsetWidth || 600;
    const targetPixels = 100;
    const targetTime = (targetPixels / containerWidth) * duration;

    const intervals = [1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200];
    let interval = intervals[intervals.length - 1];
    for (let i = 0; i < intervals.length; i++) {
      if (targetTime < intervals[i]) {
        interval = intervals[i];
        break;
      }
    }

    for (let t = 0; t <= duration; t += interval) {
      const pct = (t / duration) * 100;
      const tick = document.createElement('div');
      tick.className = 'absolute top-0 h-full border-l border-black-500/50 flex flex-col justify-end pb-1 pl-1 pointer-events-none select-none';
      tick.style.left = `${pct}%`;
      tick.innerHTML = `<span class="scale-90 origin-bottom-left text-[8px] text-black-100 font-mono font-semibold leading-none">${formatDuration(t)}</span>`;
      ruler.appendChild(tick);
    }
  };

  const renderWaveform = () => {
    const canvas = modal.querySelector('#chapters-timeline-canvas');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const wrapper = modal.querySelector('#timeline-tracks-wrapper');
    const width = wrapper.clientWidth || wrapper.offsetWidth || 600;
    const height = 64;

    canvas.width = width * window.devicePixelRatio;
    canvas.height = height * window.devicePixelRatio;
    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;
    ctx.scale(window.devicePixelRatio, window.devicePixelRatio);

    ctx.clearRect(0, 0, width, height);

    if (peaks.length === 0) return;

    const barCount = Math.min(peaks.length, Math.floor(width / 3.5));
    const gap = 1;
    const barWidth = (width - gap * (barCount - 1)) / barCount;

    ctx.fillStyle = '#4b5563';

    for (let i = 0; i < barCount; i++) {
      const peakIdx = Math.floor((i / barCount) * peaks.length);
      const peak = peaks[peakIdx];
      const barHeight = (peak / 255) * height * 0.85;
      const x = i * (barWidth + gap);
      const y = (height - barHeight) / 2;

      ctx.beginPath();
      if (ctx.roundRect) {
        ctx.roundRect(x, y, barWidth, barHeight, 0.5);
      } else {
        ctx.rect(x, y, barWidth, barHeight);
      }
      ctx.fill();
    }
  };

  const renderSegments = () => {
    const container = modal.querySelector('#chapters-segments-container');
    if (!container) return;
    container.innerHTML = '';

    currentChapters.forEach((chap, idx) => {
      const pctStart = (chap.start / duration) * 100;
      const pctEnd = (chap.end / duration) * 100;
      const widthPct = pctEnd - pctStart;

      const seg = document.createElement('div');
      seg.className = 'absolute inset-y-0 border-l border-r border-accent/40 bg-accent/5 hover:bg-accent/15 group/seg select-none flex flex-col justify-between p-1 cursor-pointer transition-colors';
      seg.style.left = `${pctStart}%`;
      seg.style.width = `${widthPct}%`;
      seg.setAttribute('data-idx', idx);

      const showText = widthPct > 6;

      seg.innerHTML = `
        <div class="absolute left-0 top-0 bottom-0 w-2 cursor-col-resize hover:bg-accent z-20 transition-colors drag-handle-left" title="Drag to adjust Start"></div>
        
        <div class="flex-grow flex flex-col justify-center items-center text-center overflow-hidden px-1 pointer-events-none">
          ${showText ? `
            <span class="text-[9px] font-bold text-accent truncate w-full seg-title">${escapeHtml(chap.title)}</span>
            <span class="text-[8px] text-white/60 font-semibold font-mono">${formatDuration(chap.start)} - ${formatDuration(chap.end)}</span>
          ` : ''}
        </div>
        
        <div class="absolute right-0 top-0 bottom-0 w-2 cursor-col-resize hover:bg-accent z-20 transition-colors drag-handle-right" title="Drag to adjust End"></div>
        
        <div class="absolute bottom-0.5 right-1 hidden group-hover/seg:flex items-center space-x-1 bg-black-600 rounded px-1 z-30">
          <button class="text-[8px] text-accent hover:underline split-seg-btn" title="Split chapter here">Split</button>
        </div>
      `;

      container.appendChild(seg);
    });

    wireSegmentEvents(container);
  };

  const wireSegmentEvents = (container) => {
    const segments = container.querySelectorAll('[data-idx]');
    segments.forEach(seg => {
      const idx = parseInt(seg.getAttribute('data-idx'), 10);

      const splitBtn = seg.querySelector('.split-seg-btn');
      if (splitBtn) {
        splitBtn.onclick = (e) => {
          e.stopPropagation();
          const chap = currentChapters[idx];
          const mid = Math.round((chap.start + chap.end) / 2);
          currentChapters.splice(idx + 1, 0, {
            title: `${chap.title} (Part 2)`,
            start: mid,
            end: chap.end
          });
          chap.end = mid;
          chap.title = `${chap.title} (Part 1)`;
          renderChaptersList();
          updateTimeline();
        };
      }

      seg.onclick = (e) => {
        const rows = modal.querySelectorAll('.chapter-row');
        rows.forEach(r => r.classList.remove('bg-accent/10', 'border-accent'));
        
        const targetRow = modal.querySelector(`.chapter-row[data-idx="${idx}"]`);
        if (targetRow) {
          targetRow.classList.add('bg-accent/10', 'border-accent');
          targetRow.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        }

        segments.forEach(s => s.classList.remove('bg-accent/25', 'border-accent'));
        seg.classList.add('bg-accent/25', 'border-accent');
      };

      const leftHandle = seg.querySelector('.drag-handle-left');
      const rightHandle = seg.querySelector('.drag-handle-right');

      const handleDragStart = (e, type) => {
        e.stopPropagation();
        e.preventDefault();

        activeDragIndex = idx;
        const isTouch = e.type.startsWith('touch');
        const touchObj = isTouch ? e.touches[0] : null;
        const startX = isTouch ? touchObj.clientX : e.clientX;
        const startY = isTouch ? touchObj.clientY : e.clientY;
        const initialStart = currentChapters[idx].start;
        const initialEnd = currentChapters[idx].end;

        const wrapper = modal.querySelector('#timeline-tracks-wrapper');
        const wrapperWidth = wrapper.clientWidth || wrapper.offsetWidth || 600;

        let tooltip = document.getElementById('timeline-drag-tooltip');
        if (!tooltip) {
          tooltip = document.createElement('div');
          tooltip.id = 'timeline-drag-tooltip';
          tooltip.className = 'fixed bg-accent text-primary font-bold px-2 py-1 rounded shadow-2xl text-[10px] pointer-events-none z-50 transition-opacity';
          document.body.appendChild(tooltip);
        }
        tooltip.style.opacity = '1';

        const updateTooltip = (clientX, clientY, timeVal) => {
          tooltip.style.left = `${clientX + 10}px`;
          tooltip.style.top = `${clientY - 30}px`;
          tooltip.textContent = `${type === 'start' ? 'Start' : 'End'}: ${formatDuration(timeVal)}`;
        };

        updateTooltip(startX, startY, type === 'start' ? initialStart : initialEnd);

        const onMouseMove = (moveEvent) => {
          const moveIsTouch = moveEvent.type.startsWith('touch');
          if (moveIsTouch && moveEvent.cancelable) {
            moveEvent.preventDefault();
          }
          const moveTouchObj = moveIsTouch ? moveEvent.touches[0] : null;
          
          if (moveIsTouch && (!moveEvent.touches || moveEvent.touches.length === 0)) return;

          const currentX = moveIsTouch ? moveTouchObj.clientX : moveEvent.clientX;
          const currentY = moveIsTouch ? moveTouchObj.clientY : moveEvent.clientY;

          const deltaX = currentX - startX;
          const deltaTime = Math.round((deltaX / wrapperWidth) * duration);

          if (type === 'start') {
            let newStart = initialStart + deltaTime;
            newStart = Math.max(0, Math.min(newStart, initialEnd - 2));

            if (idx > 0) {
              newStart = Math.max(newStart, currentChapters[idx - 1].start + 1);
            }

            currentChapters[idx].start = newStart;
            if (idx > 0) {
              currentChapters[idx - 1].end = newStart;
            }

            updateTooltip(currentX, currentY, newStart);
          } else {
            let newEnd = initialEnd + deltaTime;
            newEnd = Math.max(initialStart + 2, Math.min(newEnd, duration));

            if (idx < currentChapters.length - 1) {
              newEnd = Math.min(newEnd, currentChapters[idx + 1].end - 1);
            }

            currentChapters[idx].end = newEnd;
            if (idx < currentChapters.length - 1) {
              currentChapters[idx + 1].start = newEnd;
            }

            updateTooltip(currentX, currentY, newEnd);
          }

          const row = modal.querySelector(`.chapter-row[data-idx="${idx}"]`);
          if (row) {
            const startInput = row.querySelector('.chapter-start');
            if (startInput) startInput.value = formatDuration(currentChapters[idx].start);
            const endInput = row.querySelector('.chapter-end');
            if (endInput) endInput.value = formatDuration(currentChapters[idx].end);
          }
          if (idx > 0) {
            const prevRow = modal.querySelector(`.chapter-row[data-idx="${idx - 1}"]`);
            if (prevRow) {
              const prevEndInput = prevRow.querySelector('.chapter-end');
              if (prevEndInput) prevEndInput.value = formatDuration(currentChapters[idx - 1].end);
            }
          }
          if (idx < currentChapters.length - 1) {
            const nextRow = modal.querySelector(`.chapter-row[data-idx="${idx + 1}"]`);
            if (nextRow) {
              const nextStartInput = nextRow.querySelector('.chapter-start');
              if (nextStartInput) nextStartInput.value = formatDuration(currentChapters[idx + 1].start);
            }
          }

          updateSegmentDOM(idx);
          if (idx > 0) updateSegmentDOM(idx - 1);
          if (idx < currentChapters.length - 1) updateSegmentDOM(idx + 1);
        };

        const updateSegmentDOM = (i) => {
          const targetSeg = container.querySelector(`[data-idx="${i}"]`);
          if (!targetSeg) return;
          const chap = currentChapters[i];
          const pctStart = (chap.start / duration) * 100;
          const pctEnd = (chap.end / duration) * 100;
          const widthPct = pctEnd - pctStart;
          targetSeg.style.left = `${pctStart}%`;
          targetSeg.style.width = `${widthPct}%`;

          const labelDiv = targetSeg.querySelector('.flex-grow');
          if (labelDiv) {
            if (widthPct > 6) {
              labelDiv.innerHTML = `
                <span class="text-[9px] font-bold text-accent truncate w-full seg-title">${escapeHtml(chap.title)}</span>
                <span class="text-[8px] text-white/60 font-semibold font-mono">${formatDuration(chap.start)} - ${formatDuration(chap.end)}</span>
              `;
            } else {
              labelDiv.innerHTML = '';
            }
          }
        };

        const onMouseUp = () => {
          tooltip.style.opacity = '0';
          if (isTouch) {
            window.removeEventListener('touchmove', onMouseMove);
            window.removeEventListener('touchend', onMouseUp);
            window.removeEventListener('touchcancel', onMouseUp);
          } else {
            window.removeEventListener('mousemove', onMouseMove);
            window.removeEventListener('mouseup', onMouseUp);
          }
          currentChapters.sort((a, b) => a.start - b.start);
          renderChaptersList();
          updateTimeline();
          activeDragIndex = null;
        };

        if (isTouch) {
          window.addEventListener('touchmove', onMouseMove, { passive: false });
          window.addEventListener('touchend', onMouseUp);
          window.addEventListener('touchcancel', onMouseUp);
        } else {
          window.addEventListener('mousemove', onMouseMove);
          window.addEventListener('mouseup', onMouseUp);
        }
      };

      leftHandle.onmousedown = (e) => handleDragStart(e, 'start');
      rightHandle.onmousedown = (e) => handleDragStart(e, 'end');
      leftHandle.ontouchstart = (e) => handleDragStart(e, 'start');
      rightHandle.ontouchstart = (e) => handleDragStart(e, 'end');
    });
  };

  const updateTimeline = () => {
    renderRuler();
    renderWaveform();
    renderSegments();
  };

  const updateZoom = (level) => {
    zoomLevel = Math.max(1.0, Math.min(10.0, level));
    const zoomVal = modal.querySelector('#timeline-zoom-val');
    if (zoomVal) zoomVal.textContent = `${zoomLevel.toFixed(1)}x`;

    const wrapper = modal.querySelector('#timeline-tracks-wrapper');
    const ruler = modal.querySelector('#timeline-ruler');
    if (wrapper && ruler) {
      const widthStyle = `calc(100% * ${zoomLevel})`;
      wrapper.style.width = widthStyle;
      ruler.style.width = widthStyle;
    }

    updateTimeline();
  };

  modal.innerHTML = `
    <div class="bg-primary border border-black-400 rounded-lg max-w-3xl w-full p-6 shadow-2xl space-y-4 flex flex-col max-h-[90vh]">
      <div class="flex items-center justify-between border-b border-black-500 pb-3 flex-shrink-0">
        <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
          <span class="material-symbols text-base text-accent">toc</span>
          <span>Edit Book Chapters</span>
        </h3>
        <button id="close-edit-chapters-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-lg">close</span>
        </button>
      </div>

      <div class="bg-black-600/30 p-3 rounded border border-black-500/50 flex-shrink-0 space-y-2">
        <div class="flex items-center justify-between text-xs">
          <span class="font-semibold text-black-50 flex items-center space-x-1">
            <span class="material-symbols text-sm text-accent">insights</span>
            <span>Visual Timeline Track</span>
          </span>
          <div class="flex items-center space-x-3">
            <div class="flex items-center space-x-1">
              <span class="text-black-100 text-[10px] uppercase font-bold mr-1">Zoom:</span>
              <button id="timeline-zoom-out" class="w-6 h-6 flex items-center justify-center rounded bg-black-500 border border-black-300 hover:bg-black-400 text-white font-bold cursor-pointer text-xs select-none focus:outline-none">-</button>
              <span id="timeline-zoom-val" class="w-8 text-center text-white text-[11px] font-semibold">1.0x</span>
              <button id="timeline-zoom-in" class="w-6 h-6 flex items-center justify-center rounded bg-black-500 border border-black-300 hover:bg-black-400 text-white font-bold cursor-pointer text-xs select-none focus:outline-none">+</button>
            </div>
          </div>
        </div>

        <div id="timeline-scroll-container" class="w-full overflow-x-auto overflow-y-hidden border border-black-500 bg-black-900 rounded relative h-28 cursor-default no-scroll">
          <div id="timeline-tracks-wrapper" class="relative h-full" style="width: 100%;">
            <div id="timeline-ruler" class="absolute top-0 left-0 right-0 h-5 border-b border-black-500/40 text-[9px] text-black-100 select-none"></div>
            <canvas id="chapters-timeline-canvas" class="absolute top-5 left-0 right-0 h-16 pointer-events-none opacity-45"></canvas>
            <div id="chapters-segments-container" class="absolute top-5 bottom-0 left-0 right-0"></div>
          </div>
        </div>
      </div>

      <div class="flex items-center justify-between bg-black-600/30 p-2.5 rounded border border-black-500/50 flex-shrink-0 text-xs">
        <div class="flex items-center space-x-2">
          <button id="editor-add-chapter-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold px-2.5 py-1.5 rounded transition-colors flex items-center space-x-1">
            <span class="material-symbols text-sm">add</span>
            <span>Add Chapter</span>
          </button>
          <button id="editor-lookup-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-accent font-semibold px-2.5 py-1.5 rounded transition-colors flex items-center space-x-1">
            <span class="material-symbols text-sm">search</span>
            <span>Audnexus Lookup</span>
          </button>
        </div>
        <div class="text-black-100 text-[0.7rem]">
          ASIN: <span class="text-white font-semibold">${escapeHtml(item.media?.metadata?.asin || 'None')}</span>
        </div>
      </div>

      <div id="chapters-editor-list" class="space-y-2 overflow-y-auto no-scroll flex-grow pr-1 min-h-[180px]">
        <!-- Dynamic chapters -->
      </div>

      <div class="flex items-center justify-between pt-3 border-t border-black-500 flex-shrink-0">
        <div class="text-[0.65rem] text-black-100 flex items-center space-x-1">
          <span class="material-symbols text-xs">info</span>
          <span>Times can be entered as seconds (e.g. 120) or formats like 1:05 or 1:02:15. Adjust start/end using resize handles.</span>
        </div>
        <div class="flex items-center space-x-3">
          <button id="cancel-edit-chapters-btn" class="bg-transparent hover:bg-black-500 text-white px-4 py-2 rounded text-xs transition-colors">
            Cancel
          </button>
          <button id="save-edit-chapters-btn" class="bg-accent text-primary font-bold px-4 py-2 rounded text-xs hover:opacity-90 transition-opacity">
            Save Chapters
          </button>
        </div>
      </div>
    </div>
  `;

  document.body.appendChild(modal);
  
  document.getElementById('timeline-zoom-in').onclick = () => updateZoom(zoomLevel + 1.0);
  document.getElementById('timeline-zoom-out').onclick = () => updateZoom(zoomLevel - 1.0);

  const handleResize = () => {
    renderWaveform();
  };
  window.addEventListener('resize', handleResize);

  renderChaptersList();
  updateTimeline();

  getWaveformPeaks(item.id).then(loadedPeaks => {
    peaks = loadedPeaks;
    renderWaveform();
  });

  const closeModal = () => {
    window.removeEventListener('resize', handleResize);
    modal.remove();
  };
  
  document.getElementById('close-edit-chapters-modal').onclick = closeModal;
  document.getElementById('cancel-edit-chapters-btn').onclick = closeModal;

  document.getElementById('editor-add-chapter-btn').onclick = () => {
    let nextStart = 0;
    if (currentChapters.length > 0) {
      nextStart = currentChapters[currentChapters.length - 1].end || currentChapters[currentChapters.length - 1].start;
    }
    currentChapters.push({
      title: `New Chapter`,
      start: nextStart,
      end: nextStart + 300
    });
    renderChaptersList();
    updateTimeline();
    
    const listContainer = modal.querySelector('#chapters-editor-list');
    if (listContainer) {
      setTimeout(() => {
        listContainer.scrollTop = listContainer.scrollHeight;
      }, 50);
    }
  };

  document.getElementById('editor-lookup-btn').onclick = async () => {
    const asinVal = item.media?.metadata?.asin;
    if (!asinVal) {
      showToast("Book must have an ASIN (under Edit Details) to perform Audnexus chapter lookup.", "warning");
      return;
    }

    const btn = document.getElementById('editor-lookup-btn');
    const originalHTML = btn.innerHTML;
    btn.disabled = true;
    btn.innerHTML = `<span class="animate-spin rounded-full h-3.5 w-3.5 border-b-2 border-accent mr-1"></span> Searching...`;

    try {
      const res = await request('POST', `/api/items/${item.id}/chapters/lookup`);
      if (res && Array.isArray(res.chapters) && res.chapters.length > 0) {
        currentChapters = res.chapters;
        renderChaptersList();
        updateTimeline();
      } else {
        showToast("Audnexus lookup returned no chapters for this book.", "warning");
      }
    } catch (err) {
      console.error("Audnexus lookup failed:", err);
      showToast("Audnexus lookup failed: " + (err.message || "Unknown error"), "error");
    } finally {
      btn.disabled = false;
      btn.innerHTML = originalHTML;
    }
  };

  document.getElementById('save-edit-chapters-btn').onclick = async () => {
    for (let i = 0; i < currentChapters.length; i++) {
      const c = currentChapters[i];
      if (!c.title.trim()) {
        showToast(`Chapter ${i + 1} title cannot be empty.`, "warning");
        return;
      }
      if (c.start < 0 || c.end < 0) {
        showToast(`Chapter ${i + 1} times must be non-negative.`, "warning");
        return;
      }
      if (c.end <= c.start) {
        showToast(`Chapter ${i + 1} end time must be greater than start time.`, "warning");
        return;
      }
    }

    currentChapters.sort((a, b) => a.start - b.start);
    currentChapters.forEach((c, idx) => {
      c.id = idx + 1;
    });

    const saveBtn = document.getElementById('save-edit-chapters-btn');
    saveBtn.disabled = true;
    saveBtn.textContent = "Saving...";

    try {
      await request('POST', `/api/items/${item.id}/chapters`, {
        chapters: currentChapters
      });
      closeModal();
      if (onSaveSuccess) onSaveSuccess();
    } catch (err) {
      console.error("Failed to save chapters:", err);
      showToast("Failed to save chapters: " + (err.message || "Unknown error"), "error");
      saveBtn.disabled = false;
      saveBtn.textContent = "Save Chapters";
    }
  };
}
