/*
 * Copyright © 2026 Musing Studio LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

// Adds and removes rel="me" verification link rows on the blog settings
// page. Without JavaScript every stored row still renders and can be
// edited; only adding a new row is unavailable.
(function () {
	var container = document.getElementById('verification-links');
	var addButton = document.getElementById('add-verification');
	if (!container || !addButton) {
		return;
	}

	function newRow() {
		var row = document.createElement('div');
		row.className = 'verification-row';

		var input = document.createElement('input');
		input.type = 'text';
		input.name = 'verification_link_row';
		input.placeholder = 'https://writing.exchange/@writefreely';

		var remove = document.createElement('button');
		remove.type = 'button';
		remove.className = 'remove-verification';
		remove.title = 'Remove this link';
		remove.setAttribute('aria-label', 'Remove this link');
		remove.innerHTML = '&times;';

		row.appendChild(input);
		row.appendChild(remove);
		return row;
	}

	addButton.addEventListener('click', function () {
		var row = newRow();
		container.appendChild(row);
		row.getElementsByTagName('input')[0].focus();
	});

	// Delegated so it also covers rows added after page load.
	container.addEventListener('click', function (e) {
		if (e.target && e.target.className === 'remove-verification') {
			var row = e.target.parentNode;
			if (container.getElementsByClassName('verification-row').length > 1) {
				container.removeChild(row);
			} else {
				// Keep one row present; just clear it.
				row.getElementsByTagName('input')[0].value = '';
			}
		}
	});
})();
