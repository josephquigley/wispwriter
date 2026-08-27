/*
 * Progressive enhancement for the pinned post management page.
 *
 * The page works without this file: every control is a form that posts and
 * reloads. When scripting is available, submit the same request in the
 * background and rearrange the list in place, so reordering does not throw
 * the page away and rebuild it.
 *
 * Anything unexpected -- a failed request, a browser without the APIs this
 * needs -- falls back to submitting the form normally, so the feature
 * degrades to the behaviour it enhances rather than breaking.
 */
(function () {
	var list = document.getElementById('pinned-list');
	if (!list || !window.fetch || !window.FormData) {
		return;
	}

	var base = list.getAttribute('data-action-base');
	var busy = false;

	function rows() {
		return Array.prototype.slice.call(list.querySelectorAll('li.pinned-item'));
	}

	function csrfToken() {
		var f = list.querySelector('input[name="gorilla.csrf.Token"]');
		return f ? f.value : '';
	}

	// Build one of the move controls. The server omits the control a row
	// cannot use, so after a reorder the set has to be rebuilt.
	function moveForm(postID, action) {
		var form = document.createElement('form');
		form.method = 'post';
		form.action = base + '/' + postID + '/' + action;

		var token = document.createElement('input');
		token.type = 'hidden';
		token.name = 'gorilla.csrf.Token';
		token.value = csrfToken();

		var button = document.createElement('button');
		button.type = 'submit';
		button.className = 'pinned-move';
		button.title = action === 'up' ? 'Move up' : 'Move down';
		button.setAttribute('aria-label', button.title);
		button.innerHTML = action === 'up' ? '&#9650;' : '&#9660;';

		form.appendChild(token);
		form.appendChild(button);
		return form;
	}

	// The first row has nothing above it and the last nothing below, so
	// their controls differ from every other row's.
	function syncControls() {
		var items = rows();
		items.forEach(function (li, i) {
			var controls = li.querySelector('.pinned-controls');
			var postID = li.getAttribute('data-post-id');
			var up = controls.querySelector('form[action$="/up"]');
			var down = controls.querySelector('form[action$="/down"]');
			var unpin = controls.querySelector('form[action$="/remove"]');

			if (i === 0 && up) {
				controls.removeChild(up);
			} else if (i > 0 && !up) {
				controls.insertBefore(moveForm(postID, 'up'), controls.firstChild);
			}

			if (i === items.length - 1 && down) {
				controls.removeChild(down);
			} else if (i < items.length - 1 && !down) {
				controls.insertBefore(moveForm(postID, 'down'), unpin);
			}
		});
	}

	// Move rows with a FLIP transition: record where they are, rearrange,
	// then animate from the old position to the new one. Transforms keep
	// this off the layout path, so it stays smooth on a long list.
	function animateSwap(a, b, rearrange) {
		var beforeA = a.getBoundingClientRect().top;
		var beforeB = b.getBoundingClientRect().top;

		rearrange();

		var deltaA = beforeA - a.getBoundingClientRect().top;
		var deltaB = beforeB - b.getBoundingClientRect().top;

		[[a, deltaA], [b, deltaB]].forEach(function (pair) {
			var el = pair[0], delta = pair[1];
			el.style.transition = 'none';
			el.style.transform = 'translateY(' + delta + 'px)';
		});

		// Force a reflow so the browser treats the next change as an
		// animation rather than collapsing both into one paint.
		void list.offsetHeight;

		[a, b].forEach(function (el) {
			el.style.transition = 'transform 180ms ease';
			el.style.transform = '';
			el.addEventListener('transitionend', function done() {
				el.style.transition = '';
				el.removeEventListener('transitionend', done);
			});
		});
	}

	function collapse(li, after) {
		li.style.transition = 'opacity 120ms ease, transform 120ms ease';
		li.style.opacity = '0';
		li.style.transform = 'translateX(1em)';
		window.setTimeout(after, 120);
	}

	list.addEventListener('submit', function (e) {
		var form = e.target;
		if (form.tagName !== 'FORM' || busy) {
			return;
		}
		var li = form.closest('li.pinned-item');
		if (!li) {
			return;
		}

		e.preventDefault();
		busy = true;
		list.classList.add('is-busy');

		fetch(form.action, {
			method: 'POST',
			body: new FormData(form),
			credentials: 'same-origin',
			headers: { 'X-Requested-With': 'XMLHttpRequest' }
		}).then(function (res) {
			if (!res.ok) {
				throw new Error('request failed: ' + res.status);
			}

			if (/\/remove$/.test(form.action)) {
				collapse(li, function () {
					li.parentNode.removeChild(li);
					syncControls();
					finish();
				});
				return;
			}

			var up = /\/up$/.test(form.action);
			var other = up ? li.previousElementSibling : li.nextElementSibling;
			if (!other) {
				finish();
				return;
			}

			animateSwap(li, other, function () {
				if (up) {
					list.insertBefore(li, other);
				} else {
					list.insertBefore(other, li);
				}
			});
			syncControls();
			finish();
		}).catch(function () {
			// Let the browser do it the plain way rather than leaving the
			// page showing an order the server does not have.
			list.classList.remove('is-busy');
			form.submit();
		});

		function finish() {
			busy = false;
			list.classList.remove('is-busy');
		}
	});
})();
