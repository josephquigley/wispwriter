# Instance-wide custom CSS

WriteFreely serves an optional stylesheet at `/local/custom.css` and links it from every page it renders: the post editor, `/me`, the admin pages, login and signup, and every public blog on the instance.

It is not the **Custom CSS** box on a blog's settings page at `/me/c/<alias>`. That one is stored in the database and inlined into that blog's public pages only, so it cannot style the editor or anything else in the application, and it applies to one blog at a time. `/local/custom.css` is the only route to the editor, and the only way to set a house style across every blog at once.

Both can be live together. On a public page the per-blog rules win at equal specificity, since they are inlined further down the page than the `<link>`.

Write the file to `static/local/custom.css` under whatever `static_parent_dir` points at, which for a standard install is the directory the binary runs from. The change is live on the next page load, with no restart. The path is gitignored, so a source checkout will not offer to commit it.

In a container that directory is inside the read-only image rather than in `/data`. See [custom CSS](docker.md#custom-css) in the Docker guide for the mount to add.
