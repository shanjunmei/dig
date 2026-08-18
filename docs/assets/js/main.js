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
    // Deep-link language via ?lang=zh|en (so README doc links can open the
    // matching language). Takes precedence over any stored preference and is
    // persisted, so later visits without the param keep the chosen language.
    try {
      var param = new URLSearchParams(window.location.search).get('lang');
      if (param === 'en' || param === 'zh') {
        try { localStorage.setItem(I18N_KEY, param); } catch (e) { /* ignore */ }
        return param;
      }
    } catch (e) { /* ignore */ }
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

  // ===== Syntax highlighting (T10): unify every plain code block with the same
  // token classes the hand-written spans already use, so the whole site reads
  // consistently. Blocks that already contain <span> markup are left untouched.
  var GO_KW = new Set(('package import func var const type struct interface map chan go defer' +
    ' return if else for range switch case default break continue fallthrough select goto' +
    ' nil true false iota').split(' '));
  var GO_TYPES = new Set(('string int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64' +
    ' uintptr byte rune float32 float64 complex64 complex128 bool error interface any').split(' '));

  function escHtml(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function highlightGo(src) {
    var re = /(\/\*[\s\S]*?\*\/|\/\/[^\n]*)|("(?:\\.|[^"\\])*"|`[^`]*`|'(?:\\.|[^'\\])*')|(\b\d[\d_]*(?:\.\d+)?\b)|([A-Za-z_][A-Za-z0-9_]*)|(\s+)|([^\sA-Za-z0-9_`"])/g;
    var out = '';
    var m;
    while ((m = re.exec(src)) !== null) {
      var t = m[0];
      if (m[1]) {
        out += '<span class="c-comment">' + escHtml(t) + '</span>';
      } else if (m[2]) {
        out += '<span class="c-string">' + escHtml(t) + '</span>';
      } else if (m[3]) {
        out += '<span class="c-number">' + escHtml(t) + '</span>';
      } else if (m[4]) {
        var after = src.slice(re.lastIndex);
        var cls = '';
        if (GO_KW.has(t)) cls = 'c-keyword';
        else if (GO_TYPES.has(t)) cls = 'c-type';
        else if (/^[A-Z]/.test(t)) cls = 'c-type';
        else if (/^\s*\(/.test(after)) cls = 'c-func';
        out += cls ? '<span class="' + cls + '">' + escHtml(t) + '</span>' : escHtml(t);
      } else if (m[5]) {
        out += escHtml(t);
      } else if (m[6]) {
        out += escHtml(t);
      }
    }
    return out;
  }

  (function highlightAll() {
    document.querySelectorAll('pre').forEach(function(pre) {
      if (pre.querySelector('span')) return; // already hand-highlighted
      var code = pre.querySelector('code') || pre;
      code.innerHTML = highlightGo(code.textContent);
    });
  })();

  // ===== Table of Contents + scrollspy + search (T16) =====
  (function buildToc() {
    var toc = document.getElementById('toc');
    if (!toc) return;
    var nav = document.getElementById('toc-nav');
    var search = document.getElementById('toc-search');
    var sections = document.querySelectorAll('section[id]');
    var lang = (function() { try { return localStorage.getItem('dig-lang'); } catch (e) { return null; } })();
    var isEn = lang === 'en';
    if (search) search.placeholder = isEn ? 'Search…' : '搜索目录…';
    var links = [];
    sections.forEach(function(sec) {
      if (sec.id === 'hero') return; // hero is the landing banner, skip
      var titleEl = sec.querySelector('.section-title') || sec.querySelector('h2') || sec.querySelector('h1');
      var label = titleEl ? titleEl.textContent.trim() : sec.id;
      var a = document.createElement('a');
      a.href = '#' + sec.id;
      a.className = 'toc-link';
      a.textContent = label;
      a.dataset.label = label.toLowerCase();
      nav.appendChild(a);
      links.push(a);
    });
    if (search) {
      search.addEventListener('input', function() {
        var q = search.value.trim().toLowerCase();
        links.forEach(function(a) {
          a.style.display = (!q || a.dataset.label.indexOf(q) >= 0) ? '' : 'none';
        });
      });
    }
    if ('IntersectionObserver' in window) {
      var spy = new IntersectionObserver(function(entries) {
        entries.forEach(function(e) {
          if (e.isIntersecting) {
            var id = e.target.id;
            links.forEach(function(a) {
              a.classList.toggle('active', a.getAttribute('href') === '#' + id);
            });
          }
        });
      }, { rootMargin: '-80px 0px -70% 0px', threshold: 0 });
      sections.forEach(function(s) { spy.observe(s); });
    }
  })();

  // ===== Theme toggle (T12) =====
  (function themeToggle() {
    var KEY = 'dig-theme';
    var btn = document.getElementById('theme-toggle');
    function applyTheme(t) {
      if (t === 'dark') document.documentElement.setAttribute('data-theme', 'dark');
      else document.documentElement.setAttribute('data-theme', 'light');
      if (btn) btn.textContent = t === 'dark' ? '☀️' : '🌙';
    }
    var stored = null;
    try { stored = localStorage.getItem(KEY); } catch (e) { /* ignore */ }
    if (!stored) {
      var prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
      stored = prefersDark ? 'dark' : 'light';
    }
    applyTheme(stored);
    if (btn) {
      btn.addEventListener('click', function() {
        var cur = document.documentElement.getAttribute('data-theme');
        var next = cur === 'dark' ? 'light' : 'dark';
        applyTheme(next);
        try { localStorage.setItem(KEY, next); } catch (e) { /* ignore */ }
      });
    }
  })();

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

  // ===== Copy buttons for code blocks =====
  (function addCopyButtons() {
    var lang = null;
    try { lang = localStorage.getItem('dig-lang'); } catch (e) { /* ignore */ }
    var isEn = lang === 'en';
    document.querySelectorAll('pre').forEach(function(pre) {
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'copy-btn';
      btn.textContent = isEn ? 'Copy' : '复制';
      btn.setAttribute('aria-label', isEn ? 'Copy code' : '复制代码');
      btn.addEventListener('click', function() {
        var code = pre.querySelector('code');
        var text = code ? code.innerText : pre.innerText;
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(text).then(function() {
            btn.textContent = isEn ? 'Copied ✓' : '已复制 ✓';
            setTimeout(function() { btn.textContent = isEn ? 'Copy' : '复制'; }, 1500);
          }).catch(function() {
            btn.textContent = isEn ? 'Failed' : '复制失败';
          });
        } else {
          btn.textContent = isEn ? 'Failed' : '复制失败';
        }
      });
      pre.appendChild(btn);
    });
  })();

})();
