// UI behaviour for PeeringDB Plus. Loaded with `defer` from the layout
// head; everything binds through delegated listeners (or waits for
// DOMContentLoaded) so htmx swaps never need re-binding. This file
// replaces the inline <script> blocks the layout used to carry, which
// lets the CSP drop 'unsafe-inline' from script-src.
//
// NOTE: Tailwind classes referenced only from this file (ring-2, the
// error/retry styling, etc.) are kept in the compiled stylesheet by the
// `@source` entry for this file in internal/web/tailwind.input.css.

// Dark mode toggle. Both nav toggles share the .dark-mode-toggle class.
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
// text; the enclosing [data-copy-group] contains the .copied-msg status
// region, which shows the result for a second, and screen readers
// announce it.
(function () {
	document.addEventListener('click', function (e) {
		var el = e.target.closest('[data-copy]');
		if (!el) return;
		var group = el.closest('[data-copy-group]');
		var msg = group && group.querySelector('.copied-msg');
		function show(text) {
			if (!msg) return;
			msg.textContent = text;
			setTimeout(function () { msg.textContent = ''; }, 1000);
		}
		if (!navigator.clipboard) {
			show('Copy failed');
			return;
		}
		navigator.clipboard.writeText(el.getAttribute('data-copy')).then(
			function () { show('Copied!'); },
			function () { show('Copy failed'); }
		);
	});
})();

// Screen reader announcements for search results. The results replace
// the whole container, so a live region on it would read every row.
// A separate status element announces the number of results instead.
(function () {
	var statusFor = { 'search-results': 'search-status', 'spotlight-results': 'spotlight-status' };
	document.addEventListener('htmx:after:swap', function (e) {
		var ctx = e.detail.ctx;
		var target = ctx && ctx.target;
		var status = target && statusFor[target.id] && document.getElementById(statusFor[target.id]);
		if (!status) return;
		var source = ctx.sourceElement;
		if (!source || !source.value || !source.value.trim()) {
			status.textContent = '';
			return;
		}
		var n = target.querySelectorAll('[data-result]').length;
		status.textContent = n === 0 ? 'No results' : n === 1 ? '1 result' : n + ' results';
	});
})();

// Keyboard navigation for search results. Every result is a plain
// link in the tab order, so Tab reaches it and Enter follows it. The
// arrow keys move focus between the search input and the results, and
// Escape goes back to the input. searchResultKeys acts only while focus
// is in the input or on a result, and returns true when it handled the
// key. The spotlight overlay uses it too.
function searchResultKeys(e, input, container) {
	var options = Array.from(container.querySelectorAll('[data-result]'));
	var index = options.indexOf(document.activeElement);
	if (document.activeElement !== input && index < 0) return false;
	var target = null;
	if (e.key === 'ArrowDown') {
		target = options[Math.min(index + 1, options.length - 1)];
	} else if (e.key === 'ArrowUp') {
		target = index > 0 ? options[index - 1] : input;
	} else if (e.key === 'Escape' && index >= 0) {
		target = input;
	}
	if (!target) return false;
	e.preventDefault();
	target.focus();
	if (target !== input) target.scrollIntoView({ block: 'nearest' });
	return true;
}

(function () {
	document.addEventListener('keydown', function (e) {
		var input = document.querySelector('#search-form input[name="q"]');
		var container = document.getElementById('search-results');
		if (input && container) searchResultKeys(e, input, container);
	});
})();

// htmx error handling for collapsible sections.
// On failed fetch inside a <details> element, replaces "Loading..." with
// an error message and retry button.
(function () {
	function showSectionLoadError(evt) {
		var ctx = evt.detail && evt.detail.ctx;
		var source = ctx && ctx.sourceElement;
		var el = ctx && ctx.target;
		if (!source || !source.hasAttribute('hx-get') || !el || !el.closest('details')) return;

		var url = source.getAttribute('hx-get');
		el.textContent = '';
		var wrapper = document.createElement('div');
		wrapper.className = 'px-4 py-3 text-center';
		var msg = document.createElement('span');
		msg.className = 'text-red-700 dark:text-red-400 text-sm';
		msg.setAttribute('role', 'alert');
		msg.textContent = 'Failed to load.';
		var btn = document.createElement('button');
		btn.className = 'text-emerald-700 dark:text-emerald-400 hover:text-emerald-800 dark:hover:text-emerald-300 text-sm underline ml-2';
		btn.textContent = 'Retry';
		btn.setAttribute('hx-get', url);
		btn.setAttribute('hx-target', 'closest [data-section-loader]');
		btn.setAttribute('hx-swap', 'innerHTML');
		wrapper.appendChild(msg);
		wrapper.appendChild(btn);
		el.appendChild(wrapper);
		htmx.process(el);
	}

	document.addEventListener('htmx:response:error', showSectionLoadError);
	document.addEventListener('htmx:error', showSectionLoadError);
})();

// Client-side table sorting for sortable tables.
// Handles click on th[data-sortable], toggles asc/desc, re-orders rows.
// Each sortable header gets a button around its label, so the sort is
// reachable by keyboard, and the sorted header carries aria-sort. The
// server sends plain headers because sorting needs this script.
(function () {
	function addSortButtons(root) {
		root.querySelectorAll('th[data-sortable]').forEach(function (th) {
			if (th.querySelector('button')) return;
			var btn = document.createElement('button');
			btn.type = 'button';
			btn.className = 'sort-button';
			while (th.firstChild) btn.appendChild(th.firstChild);
			th.appendChild(btn);
		});
	}

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
			h.removeAttribute('aria-sort');
		});
		th.setAttribute('data-sort-active', dir);
		th.setAttribute('aria-sort', dir === 'asc' ? 'ascending' : 'descending');

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

	document.addEventListener('htmx:after:swap', function (e) {
		var root = e.detail.ctx.target;
		if (root) addSortButtons(root);
		applyDefaultSort(root);
	});

	document.addEventListener('DOMContentLoaded', function () {
		addSortButtons(document);
		applyDefaultSort();
	});
})();

// Spotlight search: "/" opens it, Escape closes it. The overlay is a
// modal <dialog>: showModal() makes the rest of the page inert, the
// browser closes it on Escape, and closing it returns focus to the
// element that had focus before.
(function () {
	document.addEventListener('DOMContentLoaded', function () {
		var dialog = document.getElementById('spotlight');
		var panel = document.getElementById('spotlight-panel');
		var input = document.getElementById('spotlight-input');
		var resultsContainer = document.getElementById('spotlight-results');
		var form = document.getElementById('spotlight-form');
		if (!dialog || !panel || !input || !resultsContainer || !form) return;
		if (typeof dialog.showModal !== 'function') return;

		function open() {
			input.value = '';
			resultsContainer.replaceChildren();
			var status = document.getElementById('spotlight-status');
			if (status) status.textContent = '';
			dialog.showModal();
			input.focus();
		}

		function isInputFocused() {
			var el = document.activeElement;
			return el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable);
		}

		// Numeric ASN redirect from spotlight; non-numeric submits are
		// swallowed (results arrive via the htmx input trigger instead).
		form.addEventListener('submit', function (event) {
			event.preventDefault();
			var q = input.value.trim();
			if (/^\d+$/.test(q)) {
				dialog.close();
				window.location.href = '/ui/asn/' + q;
			}
		});

		// "/" opens spotlight when not typing in a form field.
		document.addEventListener('keydown', function (e) {
			if (e.key === '/' && !dialog.open && !isInputFocused()) {
				e.preventDefault();
				open();
			}
		});

		// Escape is left to the dialog, which closes on it.
		dialog.addEventListener('keydown', function (e) {
			if (e.key !== 'Escape') searchResultKeys(e, input, resultsContainer);
		});

		// A click outside the panel lands on the dialog box, which covers
		// the viewport, and closes it.
		dialog.addEventListener('click', function (e) {
			if (!panel.contains(e.target)) dialog.close();
		});
	});
})();
