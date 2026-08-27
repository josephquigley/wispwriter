// class MarkdownView {
//   constructor(target, content) {
//     this.textarea = target.appendChild(document.createElement("textarea"))
//     this.textarea.value = content
//   }

//   get content() { return this.textarea.value }
//   focus() { this.textarea.focus() }
//   destroy() { this.textarea.remove() }
// }

import { EditorView } from "prosemirror-view";
import { EditorState, TextSelection } from "prosemirror-state";
import { exampleSetup } from "prosemirror-example-setup";
import { keymap } from "prosemirror-keymap";

import { writeFreelyMarkdownParser } from "./markdownParser";
import { writeFreelyMarkdownSerializer } from "./markdownSerializer";
import { writeFreelySchema } from "./schema";
import { getMenu } from "./menu";

let $title = document.querySelector("#title");
let $content = document.querySelector("#content");

// Bugs:
// 1. When there's just an empty line and a hard break is inserted with shift-enter then two enters are inserted
// which do not show up in the markdown ( maybe bc. they are training enters )

class ProseMirrorView {
  constructor(target, content) {
    let typingTimer;
    let localDraft = localStorage.getItem(window.draftKey);
    if (localDraft != null) {
      content = localDraft;
    }
    if (content.indexOf("# ") === 0) {
      let eol = content.indexOf("\n");
      let title = content.substring("# ".length, eol);
      content = content.substring(eol + "\n\n".length);
      $title.value = title;
    }

    const doc = writeFreelyMarkdownParser.parse(content)

    this.view = new EditorView(target, {
      state: EditorState.create({
        doc,
        plugins: [
          keymap({
            "Mod-Enter": () => {
              document.getElementById("publish").click();
              return true;
            },
            "Mod-k": () => {
              const linkButton = document.querySelector(
                ".ProseMirror-icon[title='Add or remove link']"
              );
              linkButton.dispatchEvent(new Event("mousedown"));
              return true;
            },
          }),
          ...exampleSetup({
            schema: writeFreelySchema,
            menuContent: getMenu(),
          }),
        ],
      }),
      dispatchTransaction(transaction) {
        let newState = this.state.apply(transaction);
        const newContent = writeFreelyMarkdownSerializer
          .serialize(newState.doc)
          // Replace all \\\ns ( not followed by a \n ) with \n
          .replace(/(\\\n)(\n{0,1})/g, (match, p1, p2) =>
            p2 !== "\n" ? "\n" + p2 : match
          );
        $content.value = newContent;
        let draft = "";
        if ($title.value != null && $title.value !== "") {
          draft = "# " + $title.value + "\n\n";
        }
        draft += newContent;
        clearTimeout(typingTimer);
        typingTimer = setTimeout(doneTyping, doneTypingInterval);
        this.updateState(newState);
      },
      handleDOMEvents: {
        drop: (view, event) => {
          // A file dropped in from outside is either uploaded, when the
          // instance has uploads enabled, or ignored. This does not trigger
          // when an already-inserted image is dragged to a new position.
          if (event.dataTransfer.files.length > 0) {
            event.preventDefault();
            uploadDroppedImages(view, event);
          }
        }
      },
    });
    // Editor is focused to the last position. This is a workaround for a bug:
    // 1. 1 type something in an existing entry
    // 2. reload - works fine, the draft is reloaded
    // 3. reload again - the draft is somehow removed from localStorage and the original content is loaded
    // When the editor is focused the content is re-saved to localStorage

    // This is also useful for editing, so it's not a bad thing even
    const lastPosition = this.view.state.doc.content.size;
    const selection = TextSelection.create(this.view.state.doc, lastPosition);
    this.view.dispatch(this.view.state.tr.setSelection(selection));
    this.view.focus();
  }

  get content() {
    return writeFreelyMarkdownSerializer.serialize(this.view.state.doc);
  }
  focus() {
    this.view.focus();
  }
  destroy() {
    this.view.destroy();
  }
}

// uploadsConfig is set by the editor template when the instance has image
// uploads turned on.
function uploadsConfig() {
  return window.wfImageUploads || null;
}

// uploadDroppedImages uploads each dropped file and inserts an image node at
// the drop point once it lands. ProseMirror owns its document, so the image
// arrives as a node in a transaction rather than as text.
function uploadDroppedImages(view, event) {
  const cfg = uploadsConfig();
  if (!cfg || !cfg.enabled || typeof WFImageUpload === "undefined") {
    return;
  }

  const coords = view.posAtCoords({
    left: event.clientX,
    top: event.clientY,
  });
  const pos = coords ? coords.pos : view.state.selection.from;
  const strip = document.getElementById("image-strip");

  for (let i = 0; i < event.dataTransfer.files.length; i++) {
    WFImageUpload.upload(event.dataTransfer.files[i], {
      onSuccess: (image) => {
        const node = writeFreelySchema.nodes.image.create({
          src: image.url,
          alt: image.name,
        });
        view.dispatch(view.state.tr.insert(pos, node));
        WFImageUpload.addThumbnail(strip, image);
      },
      onError: (msg) => window.alert(msg),
    });
  }
}

// removeImageNodes deletes every image node pointing at the given URL, so
// deleting a thumbnail also takes the image out of the post.
function removeImageNodes(view, url) {
  const positions = [];
  view.state.doc.descendants((node, pos) => {
    if (node.type === writeFreelySchema.nodes.image && node.attrs.src === url) {
      positions.push(pos);
    }
  });
  if (positions.length === 0) {
    return;
  }
  let tr = view.state.tr;
  // Delete from the end so earlier positions stay valid.
  positions.reverse().forEach((pos) => {
    tr = tr.delete(tr.mapping.map(pos), tr.mapping.map(pos + 1));
  });
  view.dispatch(tr);
}

let place = document.querySelector("#editor");
let view = new ProseMirrorView(place, $content.value);
window.editorView = view;

const uploads = uploadsConfig();
if (uploads && uploads.enabled && typeof WFImageUpload !== "undefined") {
  WFImageUpload.setCSRFToken(uploads.csrfToken);
  WFImageUpload.initStrip(document.getElementById("image-strip"), {
    onRemoved: (url) => removeImageNodes(view.view, url),
    onError: (msg) => window.alert(msg),
  });
}
