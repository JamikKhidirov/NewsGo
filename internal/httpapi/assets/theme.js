/* Theme switcher shared by the guide and Swagger UI.
   The initial theme is applied inline in <head> to avoid a flash. */
(function () {
  var KEY = "gonews-theme";
  var root = document.documentElement;

  function stored() {
    try {
      return localStorage.getItem(KEY);
    } catch (e) {
      return null;
    }
  }

  function systemTheme() {
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }

  function updateButtons(theme) {
    var dark = theme === "dark";
    var buttons = document.querySelectorAll("[data-theme-toggle]");
    for (var i = 0; i < buttons.length; i++) {
      var btn = buttons[i];
      btn.setAttribute("aria-pressed", dark ? "true" : "false");
      btn.title = dark ? "Включить светлую тему" : "Включить тёмную тему";
      var icon = btn.querySelector(".theme-icon");
      if (icon) {
        icon.textContent = dark ? "☀️" : "🌙";
      }
      var label = btn.querySelector(".theme-label");
      if (label) {
        label.textContent = dark ? "Светлая" : "Тёмная";
      }
    }
  }

  function apply(theme) {
    root.setAttribute("data-theme", theme);
    updateButtons(theme);
  }

  function persist(theme) {
    try {
      localStorage.setItem(KEY, theme);
    } catch (e) {
      /* private mode — theme still applies for this page */
    }
  }

  apply(stored() || systemTheme());

  document.addEventListener("click", function (event) {
    var btn = event.target.closest ? event.target.closest("[data-theme-toggle]") : null;
    if (!btn) {
      return;
    }
    var next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
    apply(next);
    persist(next);
  });

  if (window.matchMedia) {
    var mq = window.matchMedia("(prefers-color-scheme: dark)");
    var onChange = function (event) {
      if (!stored()) {
        apply(event.matches ? "dark" : "light");
      }
    };
    if (mq.addEventListener) {
      mq.addEventListener("change", onChange);
    } else if (mq.addListener) {
      mq.addListener(onChange);
    }
  }
})();
