// Theme bootstrap. Loaded synchronously in <head> so the dark class is
// set before first paint (an async/deferred load would flash the light
// theme for dark-mode users). Kept separate from ui.js, which defers.
(function () {
	var d = document.documentElement;
	var t = localStorage.getItem('darkMode');
	if (t === 'dark' || (t !== 'light' && window.matchMedia('(prefers-color-scheme:dark)').matches)) {
		d.classList.add('dark');
	}
})();
