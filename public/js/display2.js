(() => {
  const first = document.getElementById('slide-a');
  const second = document.getElementById('slide-b');
  const emptyState = document.getElementById('empty-state');

  let slides = [];
  let index = 0;
  let active = first;
  let inactive = second;
  let slideshowIntervalMs = 15_000;
  let imageRefreshMs = 60_000;

  async function loadConfig() {
    const response = await fetch('/api/config', { cache: 'no-store' });
    if (!response.ok) throw new Error(`config ${response.status}`);
    const config = await response.json();
    slideshowIntervalMs = Math.max(1, Number(config.slideshowIntervalSeconds || 15)) * 1000;
    imageRefreshMs = Math.max(5, Number(config.imageRefreshSeconds || 60)) * 1000;
  }

  async function loadImages() {
    try {
      const response = await fetch('/api/images', { cache: 'no-store' });
      if (!response.ok) throw new Error(`images ${response.status}`);
      const payload = await response.json();
      slides = Array.isArray(payload.images) ? payload.images : [];
      if (index >= slides.length) index = 0;
      renderState();
    } catch (error) {
      console.error('image list refresh failed', error);
      slides = [];
      renderState();
    }
  }

  function imageURL(name) {
    return `/images/${encodeURIComponent(name)}`;
  }

  function renderState() {
    const empty = slides.length === 0;
    emptyState.classList.toggle('hidden', !empty);
    first.classList.toggle('hidden', empty);
    second.classList.toggle('hidden', empty);
    if (!empty && !active.src) {
      active.src = imageURL(slides[index]);
      active.alt = slides[index];
    }
  }

  function advance() {
    if (slides.length === 0) return;
    index = (index + 1) % slides.length;
    const name = slides[index];
    inactive.onload = () => {
      inactive.classList.add('active');
      active.classList.remove('active');
      const previous = active;
      active = inactive;
      inactive = previous;
      inactive.onload = null;
    };
    inactive.src = imageURL(name);
    inactive.alt = name;
  }

  async function start() {
    await loadConfig();
    await loadImages();
    setInterval(advance, slideshowIntervalMs);
    setInterval(loadImages, imageRefreshMs);
  }

  start().catch((error) => {
    console.error('slideshow start failed', error);
    emptyState.classList.remove('hidden');
  });
})();
