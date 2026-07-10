// UI behaviour for PeeringDB Plus. Loaded with `defer` from the layout
// head; everything binds through delegated listeners (or waits for
// DOMContentLoaded) so htmx swaps never need re-binding. This file
// replaces the inline <script> blocks the layout used to carry, which
// lets the CSP drop 'unsafe-inline' from script-src.
//
// NOTE: Tailwind classes referenced only from this file (ring-2, the
// error/retry styling, etc.) are kept in the compiled stylesheet by the
// `@source` entry for this file in internal/web/tailwind.input.css.

// Dark mode toggle. Both nav toggles (desktop + mobile) share the
// .dark-mode-toggle class. Live-swaps map tile layers registered in
// window.__pdbMaps by map-init.js.
(function () {
	document.addEventListener('click', function (e) {
		var btn = e.target.closest('.dark-mode-toggle');
		if (!btn) return;
		var html = document.documentElement;
		html.classList.add('theme-transition');
		if (html.classList.contains('dark')) {
			html.classList.remove('dark');
			localStorage.setItem('darkMode', 'light');
		} else {
			html.classList.add('dark');
			localStorage.setItem('darkMode', 'dark');
		}
		if (window.__pdbMaps) {
			window.__pdbMaps.forEach(function (m) {
				m.tileLayer.setUrl(html.classList.contains('dark') ? m.darkURL : m.lightURL);
			});
		}
		setTimeout(function () { html.classList.remove('theme-transition'); }, 200);
	});
})();

// Mobile navigation menu: hamburger toggle plus close-on-navigate.
(function () {
	function menu() { return document.getElementById('mobile-menu'); }
	function toggleBtn() { return document.querySelector('[aria-controls="mobile-menu"]'); }

	document.addEventListener('click', function (e) {
		var m = menu();
		if (!m) return;
		if (e.target.closest('[aria-controls="mobile-menu"]')) {
			m.classList.toggle('hidden');
			toggleBtn().setAttribute('aria-expanded', m.classList.contains('hidden') ? 'false' : 'true');
			return;
		}
		if (e.target.closest('#mobile-menu a')) {
			m.classList.add('hidden');
			var btn = toggleBtn();
			if (btn) btn.setAttribute('aria-expanded', 'false');
		}
	});
})();

// Search form submit: numeric queries jump straight to the ASN detail
// page. Delegated so it also works on pages that embed SearchForm
// without the homepage (e.g. the 404 page).
(function () {
	document.addEventListener('submit', function (e) {
		var form = e.target.closest('#search-form');
		if (!form) return;
		var input = form.querySelector('input[name="q"]');
		var q = input ? input.value.trim() : '';
		if (/^\d+$/.test(q)) {
			e.preventDefault();
			window.location.href = '/ui/asn/' + q;
		}
	});
})();

// Compare form: rewrite the GET query-string submit into the canonical
// /ui/compare/{asn1}/{asn2} path form.
(function () {
	document.addEventListener('submit', function (e) {
		var form = e.target.closest('#compare-form');
		if (!form) return;
		e.preventDefault();
		var a1 = document.getElementById('compare-asn1').value;
		var a2 = document.getElementById('compare-asn2').value;
		if (a1 && a2) { window.location.href = '/ui/compare/' + a1 + '/' + a2; }
	});
})();

// Copy-to-clipboard for IP addresses. Elements carry data-copy with the
// text; the enclosing [data-copy-group] contains the .copied-msg flash.
(function () {
	document.addEventListener('click', function (e) {
		var el = e.target.closest('[data-copy]');
		if (!el) return;
		navigator.clipboard.writeText(el.getAttribute('data-copy')).then(function () {
			var group = el.closest('[data-copy-group]');
			var msg = group && group.querySelector('.copied-msg');
			if (msg) {
				msg.classList.remove('hidden');
				setTimeout(function () { msg.classList.add('hidden'); }, 1000);
			}
		});
	});
})();

// Keyboard navigation for search results.
// Listens on keydown when search input or results are focused.
(function () {
	function getOptions() {
		var container = document.getElementById('search-results');
		if (!container) return [];
		return Array.from(container.querySelectorAll('[data-result]'));
	}

	// Roving tabindex: the active link carries tabindex="0", the rest
	// stay at -1, so Tab lands on at most one result.
	function getActiveIndex(options) {
		for (var i = 0; i < options.length; i++) {
			if (options[i].getAttribute('tabindex') === '0') return i;
		}
		return -1;
	}

	function setActive(options, index) {
		for (var i = 0; i < options.length; i++) {
			options[i].setAttribute('tabindex', '-1');
			options[i].classList.remove('ring-2', 'ring-emerald-500', 'ring-offset-1');
		}
		if (index >= 0 && index < options.length) {
			options[index].setAttribute('tabindex', '0');
			options[index].classList.add('ring-2', 'ring-emerald-500', 'ring-offset-1');
			options[index].focus();
			options[index].scrollIntoView({ block: 'nearest' });
		}
	}

	document.addEventListener('keydown', function (e) {
		// Defer to spotlight handler when it is open.
		if (window.__spotlightIsOpen && window.__spotlightIsOpen()) return;

		var searchInput = document.querySelector('#search-form input[name="q"]');
		if (!searchInput) return;

		var isSearchFocused = document.activeElement === searchInput;
		var options = getOptions();
		if (options.length === 0) return;

		var currentIndex = getActiveIndex(options);

		if (e.key === 'ArrowDown') {
			e.preventDefault();
			if (isSearchFocused || currentIndex < 0) {
				setActive(options, 0);
			} else if (currentIndex < options.length - 1) {
				setActive(options, currentIndex + 1);
			}
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			if (currentIndex > 0) {
				setActive(options, currentIndex - 1);
			} else if (currentIndex === 0) {
				setActive(options, -1);
				searchInput.focus();
			}
		} else if (e.key === 'Enter') {
			if (!isSearchFocused && currentIndex >= 0) {
				e.preventDefault();
				window.location.href = options[currentIndex].getAttribute('href');
			}
		} else if (e.key === 'Escape') {
			if (!isSearchFocused) {
				e.preventDefault();
				setActive(options, -1);
				searchInput.focus();
			}
		}
	});

	document.addEventListener('htmx:afterSwap', function (e) {
		if (e.detail.target && e.detail.target.id === 'search-results') {
			var options = getOptions();
			setActive(options, -1);
		}
	});
})();

// htmx error handling for collapsible sections.
// On failed fetch inside a <details> element, replaces "Loading..." with
// an error message and retry button.
(function () {
	document.addEventListener('htmx:afterRequest', function (evt) {
		if (!evt.detail.successful && evt.detail.elt && evt.detail.elt.closest('details')) {
			var el = evt.detail.elt;
			var url = el.getAttribute('hx-get');
			el.textContent = '';
			var wrapper = document.createElement('div');
			wrapper.className = 'px-4 py-3 text-center';
			var msg = document.createElement('span');
			msg.className = 'text-red-400 text-sm';
			msg.textContent = 'Failed to load.';
			var btn = document.createElement('button');
			btn.className = 'text-emerald-400 hover:text-emerald-300 text-sm underline ml-2';
			btn.textContent = 'Retry';
			btn.setAttribute('hx-get', url);
			btn.setAttribute('hx-target', 'closest div');
			btn.setAttribute('hx-swap', 'innerHTML');
			wrapper.appendChild(msg);
			wrapper.appendChild(btn);
			el.appendChild(wrapper);
			htmx.process(el);
		}
	});
})();

// Client-side table sorting for sortable tables.
// Handles click on th[data-sortable], toggles asc/desc, re-orders rows.
(function () {
	function sortTable(th) {
		var table = th.closest('table');
		if (!table) return;
		var tbody = table.querySelector('tbody');
		if (!tbody) return;
		var col = parseInt(th.getAttribute('data-sort-col'), 10);
		var type = th.getAttribute('data-sort-type') || 'alpha';
		var current = th.getAttribute('data-sort-active');
		var dir = (current === 'asc') ? 'desc' : 'asc';

		// Clear all sort indicators in this table
		table.querySelectorAll('th[data-sortable]').forEach(function (h) {
			h.removeAttribute('data-sort-active');
		});
		th.setAttribute('data-sort-active', dir);

		var rows = Array.from(tbody.querySelectorAll('tr'));
		rows.sort(function (a, b) {
			var cellA = a.children[col];
			var cellB = b.children[col];
			var valA = cellA ? (cellA.getAttribute('data-sort-value') || '') : '';
			var valB = cellB ? (cellB.getAttribute('data-sort-value') || '') : '';

			// Empty values sort last regardless of direction
			if (valA === '' && valB !== '') return 1;
			if (valA !== '' && valB === '') return -1;
			if (valA === '' && valB === '') return 0;

			var cmp;
			if (type === 'numeric') {
				cmp = parseFloat(valA) - parseFloat(valB);
			} else {
				cmp = valA.localeCompare(valB);
			}
			return dir === 'asc' ? cmp : -cmp;
		});

		rows.forEach(function (row) { tbody.appendChild(row); });
	}

	function applyDefaultSort(root) {
		var tables = (root || document).querySelectorAll('table.sortable');
		tables.forEach(function (table) {
			var defaultTh = table.querySelector('th[data-sort-default]');
			if (defaultTh && !table.querySelector('th[data-sort-active]')) {
				sortTable(defaultTh);
			}
		});
	}

	document.addEventListener('click', function (e) {
		var th = e.target.closest('th[data-sortable]');
		if (th) sortTable(th);
	});

	document.addEventListener('htmx:afterSwap', function (e) {
		applyDefaultSort(e.detail.target);
	});

	document.addEventListener('DOMContentLoaded', function () {
		applyDefaultSort();
	});
})();

// Spotlight search: "/" to open, Escape to close.
(function () {
	document.addEventListener('DOMContentLoaded', function () {
		var overlay = document.getElementById('spotlight-overlay');
		var backdrop = document.getElementById('spotlight-backdrop');
		var input = document.getElementById('spotlight-input');
		var resultsContainer = document.getElementById('spotlight-results');
		var form = document.getElementById('spotlight-form');
		if (!overlay || !backdrop || !input || !resultsContainer || !form) return;

		function isOpen() {
			return overlay.classList.contains('open');
		}

		function open() {
			overlay.classList.remove('hidden');
			// Force reflow so the transition triggers.
			overlay.offsetHeight;
			overlay.classList.add('open');
			input.value = '';
			resultsContainer.replaceChildren();
			input.focus();
		}

		function close() {
			overlay.classList.remove('open');
			setTimeout(function () { overlay.classList.add('hidden'); }, 150);
		}

		function isInputFocused() {
			var el = document.activeElement;
			return el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable);
		}

		// Numeric ASN redirect from spotlight; non-numeric submits are
		// swallowed (results arrive via the htmx input trigger instead).
		form.addEventListener('submit', function (event) {
			event.preventDefault();
			var q = input.value.trim();
			if (/^\d+$/.test(q)) {
				close();
				window.location.href = '/ui/asn/' + q;
			}
		});

		// Keyboard navigation for spotlight results.
		function getOptions() {
			return Array.from(resultsContainer.querySelectorAll('[data-result]'));
		}

		// Roving tabindex, mirroring the page-level keyboard nav above.
		function getActiveIndex(options) {
			for (var i = 0; i < options.length; i++) {
				if (options[i].getAttribute('tabindex') === '0') return i;
			}
			return -1;
		}

		function setActive(options, index) {
			for (var i = 0; i < options.length; i++) {
				options[i].setAttribute('tabindex', '-1');
				options[i].classList.remove('ring-2', 'ring-emerald-500', 'ring-offset-1');
			}
			if (index >= 0 && index < options.length) {
				options[index].setAttribute('tabindex', '0');
				options[index].classList.add('ring-2', 'ring-emerald-500', 'ring-offset-1');
				options[index].focus();
				options[index].scrollIntoView({ block: 'nearest' });
			}
		}

		document.addEventListener('keydown', function (e) {
			// "/" opens spotlight when not typing in an input.
			if (e.key === '/' && !isOpen() && !isInputFocused()) {
				e.preventDefault();
				open();
				return;
			}

			if (!isOpen()) return;

			// Escape closes spotlight.
			if (e.key === 'Escape') {
				e.preventDefault();
				close();
				return;
			}

			var options = getOptions();
			if (options.length === 0) return;
			var currentIndex = getActiveIndex(options);
			var isInInput = document.activeElement === input;

			if (e.key === 'ArrowDown') {
				e.preventDefault();
				if (isInInput || currentIndex < 0) {
					setActive(options, 0);
				} else if (currentIndex < options.length - 1) {
					setActive(options, currentIndex + 1);
				}
			} else if (e.key === 'ArrowUp') {
				e.preventDefault();
				if (currentIndex > 0) {
					setActive(options, currentIndex - 1);
				} else if (currentIndex === 0) {
					setActive(options, -1);
					input.focus();
				}
			} else if (e.key === 'Enter' && !isInInput && currentIndex >= 0) {
				e.preventDefault();
				close();
				window.location.href = options[currentIndex].getAttribute('href');
			}
		});

		// Close on backdrop click.
		backdrop.addEventListener('click', close);

		// Reset selection after htmx swaps in new results.
		document.addEventListener('htmx:afterSwap', function (e) {
			if (e.detail.target && e.detail.target.id === 'spotlight-results') {
				var options = getOptions();
				setActive(options, -1);
			}
		});

		// Expose isOpen for the homepage search nav guard.
		window.__spotlightIsOpen = isOpen;
	});
})();
