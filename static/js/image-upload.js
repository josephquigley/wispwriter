/*
 * Copyright © 2026 Musing Studio LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 *
 * Drag-and-drop image uploads for the editors. A placeholder link is
 * inserted at the cursor the moment a file is dropped, and rewritten with
 * upload progress until the real URL replaces it -- the approach started in
 * upstream's image-upload-drag branch.
 */

var WFImageUpload = (function () {
	'use strict';

	var UPLOAD_URL = '/api/me/images';

	// csrfToken is set by init() and sent with every request.
	var csrfToken = '';

	function randomID() {
		return Math.random().toString(36).substring(2, 10);
	}

	function escapeRegExp(s) {
		return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
	}

	// baseName strips a file extension, for use as the image's alt text.
	function baseName(name) {
		var dot = name.lastIndexOf('.');
		return dot > 0 ? name.substring(0, dot) : name;
	}

	// placeholderTarget is the link target a not-yet-finished upload uses. It
	// carries the upload's identifier so concurrent uploads never rewrite
	// each other's text.
	function placeholderTarget(uploadID, percent) {
		return '#uploading-' + uploadID + '-' + percent + '%';
	}

	function placeholderRegExp(uploadID) {
		return new RegExp('!\\[[^\\]]*\\]\\(#uploading-' + escapeRegExp(uploadID) + '-[^)]*\\)');
	}

	// upload sends one file and reports back through the given handlers.
	// XMLHttpRequest is used rather than fetch because only it exposes
	// upload progress.
	function upload(file, handlers) {
		var xhr = new XMLHttpRequest();
		xhr.open('POST', UPLOAD_URL);
		xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
		if (csrfToken) {
			xhr.setRequestHeader('X-CSRF-Token', csrfToken);
		}

		xhr.upload.onprogress = function (e) {
			if (e.lengthComputable && handlers.onProgress) {
				handlers.onProgress(Math.floor((e.loaded / e.total) * 100));
			}
		};

		xhr.onreadystatechange = function () {
			if (xhr.readyState !== 4) {
				return;
			}
			if (xhr.status === 200) {
				var res;
				try {
					res = JSON.parse(xhr.responseText);
				} catch (e) {
					handlers.onError('The server sent back something unreadable.');
					return;
				}
				handlers.onSuccess(res.data || res);
				return;
			}
			handlers.onError(errorMessage(xhr));
		};

		var form = new FormData();
		form.append('file', file);
		xhr.send(form);
	}

	function errorMessage(xhr) {
		switch (xhr.status) {
			case 0:
				return "Couldn't reach the server.";
			case 413:
				return 'That image is too large.';
			case 415:
				return "That file isn't an image we can store.";
			case 507:
				return "The server couldn't store that image.";
		}
		return 'Upload failed (' + xhr.status + ').';
	}

	// remove deletes an image server-side.
	function remove(imageID, handlers) {
		var xhr = new XMLHttpRequest();
		xhr.open('DELETE', UPLOAD_URL + '/' + encodeURIComponent(imageID));
		xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
		if (csrfToken) {
			xhr.setRequestHeader('X-CSRF-Token', csrfToken);
		}
		xhr.onreadystatechange = function () {
			if (xhr.readyState !== 4) {
				return;
			}
			if (xhr.status === 204) {
				handlers.onSuccess();
			} else if (xhr.status === 409) {
				handlers.onError('Another post still uses this image, so it was kept.');
			} else {
				handlers.onError(errorMessage(xhr));
			}
		};
		xhr.send();
	}

	// replaceInTextarea swaps text without losing the cursor position, which
	// assigning to value would otherwise reset.
	function replaceInTextarea(textarea, pattern, replacement) {
		var start = textarea.selectionStart;
		var end = textarea.selectionEnd;
		textarea.value = textarea.value.replace(pattern, replacement);
		textarea.selectionStart = start;
		textarea.selectionEnd = end;
	}

	function insertAtCursor(textarea, text) {
		var start = textarea.selectionStart;
		var before = textarea.value.substring(0, start);
		var after = textarea.value.substring(textarea.selectionEnd);
		textarea.value = before + text + after;
		textarea.selectionStart = textarea.selectionEnd = start + text.length;
	}

	// initStrip wires the thumbnail strip's delete controls. onRemoved is
	// called with the deleted image's URL so each editor can take the link
	// out of its own document.
	function initStrip(strip, handlers) {
		if (!strip) {
			return;
		}
		strip.addEventListener('click', function (e) {
			var btn = e.target;
			if (!btn || btn.className !== 'remove-image') {
				return;
			}
			e.preventDefault();

			var thumb = btn.parentNode;
			var imageID = thumb.getAttribute('data-image-id');
			var url = thumb.getAttribute('data-image-url');

			remove(imageID, {
				onSuccess: function () {
					if (handlers.onRemoved) {
						handlers.onRemoved(url);
					}
					var strip = thumb.parentNode;
					strip.removeChild(thumb);
					syncStripState(strip);
				},
				onError: handlers.onError
			});
		});
	}

	// stripLinks removes every markdown link pointing at the given URL from
	// a textarea's contents.
	function stripLinks(textarea, url) {
		if (!textarea || !url) {
			return;
		}
		var pattern = new RegExp('!?\\[[^\\]]*\\]\\(' + escapeRegExp(url) + '\\)', 'g');
		replaceInTextarea(textarea, pattern, '');
	}

	// syncStripState marks the document while the strip holds anything, so
	// the stylesheet can shorten the editor by exactly the strip's height
	// rather than letting the two overlap.
	function syncStripState(strip) {
		if (!strip) {
			return;
		}
		var has = strip.getElementsByClassName('uploaded-image').length > 0;
		var cls = document.body.className.replace(/\s*has-images\b/, '');
		document.body.className = has ? (cls + ' has-images').replace(/^\s+/, '') : cls;
	}

	// addThumbnail appends an image to the strip.
	function addThumbnail(strip, image) {
		if (!strip) {
			return;
		}
		var thumb = document.createElement('span');
		thumb.className = 'uploaded-image';
		thumb.setAttribute('data-image-id', image.id);
		thumb.setAttribute('data-image-url', image.url);

		var img = document.createElement('img');
		img.src = image.url;
		img.alt = image.name;
		thumb.appendChild(img);

		var btn = document.createElement('a');
		btn.href = '#';
		btn.className = 'remove-image';
		btn.title = 'Remove this image';
		btn.appendChild(document.createTextNode('✕'));
		thumb.appendChild(btn);

		strip.appendChild(thumb);
		syncStripState(strip);
	}

	// init wires drag-and-drop uploads into a textarea-based editor.
	function init(opts) {
		csrfToken = opts.csrfToken || '';

		var textarea = opts.textarea;
		var strip = opts.strip;
		var onError = opts.onError || function (msg) {
			window.alert(msg);
		};

		initStrip(strip, {
			onRemoved: function (url) {
				stripLinks(textarea, url);
			},
			onError: onError
		});

		// A post being edited may already have images, rendered with the
		// page, so the editor has to start shortened rather than only
		// after the first upload.
		syncStripState(strip);

		if (!textarea) {
			return;
		}

		var dropArea = opts.dropArea || textarea;
		// Both are needed to stop the browser dropping the file:// URI into
		// the text itself.
		dropArea.addEventListener('dragenter', function (e) {
			e.preventDefault();
		});
		dropArea.addEventListener('dragover', function (e) {
			e.preventDefault();
		});
		dropArea.addEventListener('drop', function (e) {
			if (!e.dataTransfer || !e.dataTransfer.files || !e.dataTransfer.files.length) {
				return;
			}
			e.preventDefault();
			for (var i = 0; i < e.dataTransfer.files.length; i++) {
				uploadIntoTextarea(textarea, strip, e.dataTransfer.files[i], onError);
			}
		});
	}

	function uploadIntoTextarea(textarea, strip, file, onError) {
		var uploadID = randomID();
		var alt = baseName(file.name);

		insertAtCursor(textarea, '![' + alt + '](' + placeholderTarget(uploadID, 0) + ')');

		upload(file, {
			onProgress: function (percent) {
				replaceInTextarea(textarea, placeholderRegExp(uploadID),
					'![' + alt + '](' + placeholderTarget(uploadID, percent) + ')');
			},
			onSuccess: function (image) {
				replaceInTextarea(textarea, placeholderRegExp(uploadID),
					'![' + alt + '](' + image.url + ')');
				addThumbnail(strip, image);
			},
			onError: function (msg) {
				// Never leave a dead link behind.
				replaceInTextarea(textarea, placeholderRegExp(uploadID), '');
				onError(msg);
			}
		});
	}

	return {
		init: init,
		initStrip: initStrip,
		stripLinks: stripLinks,
		upload: upload,
		remove: remove,
		addThumbnail: addThumbnail,
		setCSRFToken: function (t) {
			csrfToken = t || '';
		}
	};
})();
