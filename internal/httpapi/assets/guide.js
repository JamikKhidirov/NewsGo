/* Guide enhancements: mobile drawer, external links, table wrapping,
   copy buttons and TOC scroll highlighting. */
(function () {
  "use strict";

  var sidebar = document.getElementById("sidebar");
  var backdrop = document.getElementById("sidebar-backdrop");
  var menu = document.getElementById("menu-toggle");

  function closeSidebar() {
    if (!sidebar) return;
    sidebar.classList.remove("open");
    if (backdrop) backdrop.hidden = true;
    if (menu) menu.setAttribute("aria-expanded", "false");
  }

  if (menu && sidebar) {
    menu.addEventListener("click", function () {
      var open = !sidebar.classList.contains("open");
      sidebar.classList.toggle("open", open);
      if (backdrop) backdrop.hidden = !open;
      menu.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }

  if (backdrop) {
    backdrop.addEventListener("click", closeSidebar);
  }

  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape") closeSidebar();
  });

  // Open external links in a new tab.
  var links = document.querySelectorAll('.markdown a[href^="http"]');
  for (var i = 0; i < links.length; i++) {
    links[i].target = "_blank";
    links[i].rel = "noopener noreferrer";
  }

  // Wrap wide tables so they scroll instead of overflowing the page.
  var tables = document.querySelectorAll(".markdown table");
  for (var t = 0; t < tables.length; t++) {
    var table = tables[t];
    if (table.parentElement && table.parentElement.classList.contains("table-wrap")) {
      continue;
    }
    var wrap = document.createElement("div");
    wrap.className = "table-wrap";
    table.parentNode.insertBefore(wrap, table);
    wrap.appendChild(table);
  }

  // Add a copy button to every code block.
  var blocks = document.querySelectorAll(".markdown pre");
  for (var b = 0; b < blocks.length; b++) {
    addCopyButton(blocks[b]);
  }

  function addCopyButton(pre) {
    var button = document.createElement("button");
    button.type = "button";
    button.className = "copy-btn";
    button.textContent = "Копировать";
    button.addEventListener("click", function () {
      var code = pre.querySelector("code") || pre;
      var text = code.innerText;
      var done = function () {
        button.textContent = "Скопировано";
        setTimeout(function () {
          button.textContent = "Копировать";
        }, 1500);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, done);
        return;
      }
      var area = document.createElement("textarea");
      area.value = text;
      area.setAttribute("readonly", "");
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      try {
        document.execCommand("copy");
        done();
      } catch (e) {
        /* ignore */
      }
      document.body.removeChild(area);
    });
    pre.appendChild(button);
  }

  // Highlight the TOC entry of the section currently in view.
  var tocLinks = Array.prototype.slice.call(
    document.querySelectorAll('.docs-toc a[href^="#"]')
  );
  if (!tocLinks.length || !("IntersectionObserver" in window)) {
    return;
  }

  var byId = {};
  tocLinks.forEach(function (link) {
    var id = decodeURIComponent(link.getAttribute("href").slice(1));
    byId[id] = link;
  });

  var headings = document.querySelectorAll(".markdown h2[id], .markdown h3[id]");
  if (!headings.length) {
    return;
  }

  var observer = new IntersectionObserver(
    function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        tocLinks.forEach(function (link) {
          link.classList.remove("active");
        });
        var active = byId[entry.target.id];
        if (active) active.classList.add("active");
      });
    },
    { rootMargin: "-84px 0px -72% 0px", threshold: 0 }
  );

  for (var h = 0; h < headings.length; h++) {
    observer.observe(headings[h]);
  }
})();
