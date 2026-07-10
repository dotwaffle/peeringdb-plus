// Leaflet map bootstrap. Loaded synchronously in the layout head right
// after leaflet.markercluster.js (only on map-bearing pages), so the L
// global is guaranteed to exist. Map containers declare themselves with
// data-map="single" or data-map="multi" plus data attributes carrying
// the server-rendered marker payload; this file scans and initializes
// them on DOMContentLoaded. Replaces the former inline templ script
// components so the CSP can drop 'unsafe-inline' from script-src.

// Leaflet's CircleMarker has no setOpacity, which markercluster calls
// during cluster animations; map both opacities onto setStyle.
L.CircleMarker.include({
	setOpacity: function (opacity) {
		this.setStyle({ opacity: opacity, fillOpacity: opacity });
	}
});

(function () {
	var LIGHT_URL = 'https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png';
	var DARK_URL = 'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png';
	var ATTRIBUTION = '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>';

	// newBaseMap creates the map + theme-aware tile layer and registers
	// the layer in window.__pdbMaps for the dark-mode toggle in ui.js.
	function newBaseMap(el) {
		var isDark = document.documentElement.classList.contains('dark');
		var map = L.map(el, { scrollWheelZoom: false });
		var tileLayer = L.tileLayer(isDark ? DARK_URL : LIGHT_URL, {
			attribution: ATTRIBUTION,
			subdomains: 'abcd',
			maxZoom: 19
		}).addTo(map);

		map.on('click', function () { map.scrollWheelZoom.enable(); });
		map.on('mouseout', function () { map.scrollWheelZoom.disable(); });

		window.__pdbMaps = window.__pdbMaps || [];
		window.__pdbMaps.push({ tileLayer: tileLayer, lightURL: LIGHT_URL, darkURL: DARK_URL });
		return map;
	}

	// Single-marker map: facility detail page.
	function initSingleMap(el) {
		var lat = parseFloat(el.getAttribute('data-lat'));
		var lng = parseFloat(el.getAttribute('data-lng'));
		var zoom = parseInt(el.getAttribute('data-zoom'), 10);
		var map = newBaseMap(el);
		map.setView([lat, lng], zoom);
		// data-popup carries server-built, server-escaped HTML.
		L.marker([lat, lng]).addTo(map).bindPopup(el.getAttribute('data-popup'));
	}

	// Multi-pin clustered map: network/IX detail and compare pages.
	function initMultiPinMap(el) {
		var markers = JSON.parse(el.getAttribute('data-markers'));
		if (!markers || markers.length === 0) return;

		var map = newBaseMap(el);
		var clusterGroup = L.markerClusterGroup();
		var bounds = [];

		for (var i = 0; i < markers.length; i++) {
			var m = markers[i];
			var cm = L.circleMarker([m.lat, m.lng], {
				radius: 7,
				fillColor: m.color,
				color: m.stroke,
				weight: 2,
				opacity: 1,
				fillOpacity: 0.8
			}).bindPopup(m.popup);
			clusterGroup.addLayer(cm);
			bounds.push([m.lat, m.lng]);
		}

		map.addLayer(clusterGroup);
		map.fitBounds(bounds, { maxZoom: 13, padding: [20, 20] });

		var legendJSON = el.getAttribute('data-legend');
		if (legendJSON) {
			var legend = JSON.parse(legendJSON);
			var isDark = document.documentElement.classList.contains('dark');
			var ctrl = L.control({ position: 'bottomleft' });
			ctrl.onAdd = function () {
				var div = L.DomUtil.create('div', '');
				var bg = isDark ? 'rgba(38,38,38,0.92)' : 'rgba(255,255,255,0.92)';
				var textColor = isDark ? '#d4d4d4' : '#171717';
				var shadow = isDark ? '0 1px 3px rgba(0,0,0,0.3)' : '0 1px 3px rgba(0,0,0,0.1)';
				div.style.cssText = 'background:' + bg + ';padding:8px 12px;border-radius:6px;font-size:12px;line-height:1.5;box-shadow:' + shadow + ';color:' + textColor;
				// Legend content uses server-side labels that are not user-provided
				// input (static ASN identifiers set by the Go template).
				function row(color, label) {
					return '<div style="display:flex;align-items:center;gap:4px;margin-bottom:2px;"><svg width="10" height="10"><circle cx="5" cy="5" r="5" fill="' + color + '"/></svg><span>' + label + '</span></div>';
				}
				div.innerHTML = row('#10b981', legend.shared) + row('#38bdf8', legend.netA) + row('#f59e0b', legend.netB);
				return div;
			};
			ctrl.addTo(map);
		}
	}

	document.addEventListener('DOMContentLoaded', function () {
		document.querySelectorAll('[data-map]').forEach(function (el) {
			if (el.hasAttribute('data-map-ready')) return;
			el.setAttribute('data-map-ready', '');
			if (el.getAttribute('data-map') === 'single') {
				initSingleMap(el);
			} else {
				initMultiPinMap(el);
			}
		});
	});
})();
