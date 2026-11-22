import path from 'node:path';

// Rewrites relative links to .md files (kept relative so GitHub renders them) into site URLs.
export default function rehypeMdLinks({ base }) {
  const root = path.resolve('src/content/docs');
  return (tree, file) => {
    const from = path.dirname(file.path);
    (function walk(node) {
      const href = node.type === 'element' && node.tagName === 'a' ? node.properties?.href : undefined;
      const m = typeof href === 'string' && href.match(/^(?![a-z][a-z0-9+.-]*:|\/|#)([^#?]+)\.md(#.*)?$/i);
      if (m) {
        const target = path.relative(root, path.resolve(from, m[1]));
        if (target.startsWith('..')) throw new Error(`${file.path}: link ${href} leaves docs/src/content/docs`);
        node.properties.href = `${base}/${target}/${m[2] ?? ''}`;
      }
      node.children?.forEach(walk);
    })(tree);
  };
}
