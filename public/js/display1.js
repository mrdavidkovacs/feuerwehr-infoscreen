(() => {
  const STATUS_INTERVAL_MS = 30_000;
  const RELOAD_INTERVAL_MS = 60 * 60 * 1000;

  const frame = document.getElementById('main-frame');
  const fallback = document.getElementById('fallback');
  const lastSuccessEl = document.getElementById('last-success');
  const currentTimeEl = document.getElementById('current-time');

  const dateTimeFormatter = new Intl.DateTimeFormat('de-AT', {
    dateStyle: 'medium',
    timeStyle: 'medium',
  });

  function formatDate(value) {
    if (!value) return 'Noch keine erfolgreiche Verbindung';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return 'Unbekannt';
    return dateTimeFormatter.format(date);
  }

  function updateClock() {
    currentTimeEl.textContent = dateTimeFormatter.format(new Date());
  }

  function setOnline(online, lastSuccess) {
    if (lastSuccess) lastSuccessEl.dataset.lastSuccess = lastSuccess;
    lastSuccessEl.textContent = formatDate(lastSuccess || lastSuccessEl.dataset.lastSuccess || '');
    frame.classList.toggle('hidden', !online);
    fallback.classList.toggle('hidden', online);
  }

  async function loadConfig() {
    const response = await fetch('/api/config', { cache: 'no-store' });
    if (!response.ok) throw new Error(`config ${response.status}`);
    return response.json();
  }

  async function checkStatus() {
    try {
      const response = await fetch('/api/status', { cache: 'no-store' });
      if (!response.ok) throw new Error(`status ${response.status}`);
      const status = await response.json();
      setOnline(Boolean(status.online), status.lastSuccess);
    } catch (error) {
      console.error('status check failed', error);
      setOnline(false, lastSuccessEl.dataset.lastSuccess || '');
    }
  }

  async function start() {
    updateClock();
    setInterval(updateClock, 1000);
    setTimeout(() => window.location.reload(), RELOAD_INTERVAL_MS);

    try {
      const config = await loadConfig();
      frame.src = config.mainUrl;
    } catch (error) {
      console.error('config loading failed', error);
      setOnline(false, '');
      return;
    }

    await checkStatus();
    setInterval(checkStatus, STATUS_INTERVAL_MS);
  }

  start();
})();
