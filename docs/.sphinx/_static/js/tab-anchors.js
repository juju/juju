(() => {
  function revealTabAnchor() {
    let id;
    try {
      id = decodeURIComponent(window.location.hash.slice(1));
    } catch {
      return;
    }

    const target = document.getElementById(id);
    const panel = target?.closest(".sphinx-tabs-panel[hidden]");
    if (!panel) return;

    const tab = document.getElementById(panel.getAttribute("aria-labelledby"));
    if (!tab) return;

    tab.click();
    target.scrollIntoView();
  }

  // Wait for sphinx-tabs to attach its click handlers.
  window.addEventListener("DOMContentLoaded", () => requestAnimationFrame(revealTabAnchor));
  window.addEventListener("hashchange", revealTabAnchor);
})();
