(function() {
  'use strict';

  // ===== i18n: bilingual (zh / en) switching =====
  var I18N_KEY = 'dig-lang';
  var META_DESC = {
    zh: 'dig — 编译期依赖注入 for Go。Fx 风格极简 API + Wire 风格代码生成，零运行时反射，零运行时依赖。',
    en: 'dig — compile-time dependency injection for Go. Fx-style minimal API + Wire-style code generation. Zero reflection, zero runtime dependency.'
  };
  var TITLE = {
    zh: 'dig — 编译期依赖注入 for Go',
    en: 'dig — Compile-time DI for Go'
  };

  function getLang() {
    var stored = null;
    try { stored = localStorage.getItem(I18N_KEY); } catch (e) { /* ignore */ }
    return (stored === 'en' || stored === 'zh') ? stored : 'zh';
  }

  function setLang(lang) {
    try { localStorage.setItem(I18N_KEY, lang); } catch (e) { /* ignore */ }
    applyLang(lang);
  }

  function applyLang(lang) {
    var isEn = lang === 'en';
    document.documentElement.lang = isEn ? 'en' : 'zh-CN';

    document.querySelectorAll('[data-en]').forEach(function(el) {
      el.textContent = isEn ? el.dataset.en : el.dataset.zh;
    });
    document.querySelectorAll('[data-en-html]').forEach(function(el) {
      el.innerHTML = isEn ? el.dataset.enHtml : el.dataset.zhHtml;
    });

    document.title = TITLE[lang];
    var meta = document.querySelector('meta[name="description"]');
    if (meta) meta.setAttribute('content', META_DESC[lang]);

    var switchBtns = document.querySelectorAll('.lang-switch button');
    switchBtns.forEach(function(b) {
      var active = b.dataset.lang === lang;
      b.classList.toggle('active', active);
      b.setAttribute('aria-pressed', active ? 'true' : 'false');
    });
  }

  // ===== i18n bootstrap =====
  // Snapshots the original (Chinese) text once, then wires the switch and
  // applies the stored language. INVARIANT: captureOriginal() MUST run before
  // any call to applyLang(); otherwise dataset.zh would be captured from an
  // already-swapped (English) value, and switching back to zh would show wrong
  // text. The script sits at end of <body>, so this runs synchronously during
  // parse — no flash of untranslated Chinese for EN users.
  function captureOriginal() {
    document.querySelectorAll('[data-en]').forEach(function(el) {
      if (el.dataset.zh === undefined) el.dataset.zh = el.textContent;
    });
    document.querySelectorAll('[data-en-html]').forEach(function(el) {
      if (el.dataset.zhHtml === undefined) el.dataset.zhHtml = el.innerHTML;
    });
  }

  function initI18n() {
    captureOriginal(); // 1) snapshot Chinese originals — must come first
    document.querySelectorAll('.lang-switch button').forEach(function(b) {
      b.addEventListener('click', function() { setLang(b.dataset.lang); });
    });
    applyLang(getLang()); // 2) apply stored language last
  }

  initI18n();

  // ===== Navbar scroll shadow =====
  var navbar = document.getElementById('navbar');
  window.addEventListener('scroll', function() {
    if (window.scrollY > 10) {
      navbar.classList.add('scrolled');
    } else {
      navbar.classList.remove('scrolled');
    }
  });

  // ===== Mobile menu toggle =====
  var navToggle = document.getElementById('nav-toggle');
  var navLinks = document.getElementById('nav-links');
  if (navToggle) {
    navToggle.addEventListener('click', function() {
      navToggle.classList.toggle('active');
      navLinks.classList.toggle('active');
    });
    navLinks.querySelectorAll('a').forEach(function(link) {
      link.addEventListener('click', function() {
        navToggle.classList.remove('active');
        navLinks.classList.remove('active');
      });
    });
  }

  // ===== Comparison tabs =====
  var tabs = document.querySelectorAll('.comp-tab');
  var panels = document.querySelectorAll('.comp-panel');
  tabs.forEach(function(tab) {
    tab.addEventListener('click', function() {
      var target = tab.getAttribute('data-tab');
      tabs.forEach(function(t) { t.classList.remove('active'); });
      panels.forEach(function(p) { p.classList.remove('active'); });
      tab.classList.add('active');
      var panel = document.getElementById('comp-' + target);
      if (panel) panel.classList.add('active');
    });
  });

  // ===== Smooth scroll offset fix (for older browsers) =====
  document.querySelectorAll('a[href^="#"]').forEach(function(anchor) {
    anchor.addEventListener('click', function(e) {
      var href = this.getAttribute('href');
      if (href === '#' || href.length < 2) return;
      var target = document.querySelector(href);
      if (target) {
        e.preventDefault();
        var offset = 64; // navbar height
        var top = target.getBoundingClientRect().top + window.scrollY - offset;
        window.scrollTo({ top: top, behavior: 'smooth' });
      }
    });
  });

})();
