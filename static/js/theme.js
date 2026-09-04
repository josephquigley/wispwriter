// Theme handling shared by the editor surfaces: the pad, the classic editor
// and the post metadata editor. All three store one preference, under
// `padTheme`, so all three have to resolve it identically. They did not, and
// an author who had never touched the toggle got a pad that followed the
// operating system and a metadata editor that forced light.
//
// Only an explicit toggle is stored. An absent preference means "follow the
// OS", which is why nothing here writes on the resolve path: a write there
// replaces the unset preference the first time a page loads, pinning the
// editor to whatever the OS happened to be at that moment.
//
// Requires h.js, for H.get and H.set.

// setTheme applies a theme to the page without storing it.
function setTheme(newTheme) {
	document.body.classList.remove('light');
	document.body.classList.remove('dark');
	document.body.classList.add(newTheme);

	var $tools = document.getElementById('tools');
	if (!$tools) {
		return;
	}
	// Icons ship in two variants, and `_dark@2x.png` is the dark glyph the
	// light theme uses. Normalising to the base name before re-adding the
	// suffix keeps this idempotent, so repeated calls cannot produce
	// `_dark_dark@2x.png`.
	var btns = Array.prototype.slice.call($tools.querySelectorAll('a img'));
	for (var i = 0; i < btns.length; i++) {
		btns[i].src = btns[i].src.replace('_dark@2x.png', '@2x.png');
		if (newTheme == 'light') {
			btns[i].src = btns[i].src.replace('@2x.png', '_dark@2x.png');
		}
	}
}

// toggleTheme switches the theme and stores the result, which is what takes
// the editor off the OS and onto an explicit choice.
function toggleTheme() {
	var newTheme = document.body.classList.contains('light') ? 'dark' : 'light';
	setTheme(newTheme);
	H.set('padTheme', newTheme);
}

function systemTheme() {
	return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

(function () {
	var padTheme = H.get('padTheme', 'auto');
	if (padTheme == 'light' || padTheme == 'dark') {
		setTheme(padTheme);
	} else {
		setTheme(systemTheme());
		// No explicit choice, so keep following the OS while the page is open.
		window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function () {
			if (H.get('padTheme', 'auto') == 'auto') {
				setTheme(systemTheme());
			}
		});
	}

	var toggle = document.getElementById('toggle-theme');
	if (toggle) {
		toggle.addEventListener('click', function (e) {
			e.preventDefault();
			toggleTheme();
		});
	}
})();
