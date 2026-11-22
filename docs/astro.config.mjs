import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import rehypeMdLinks from './src/rehype-md-links.mjs';

export default defineConfig({
  site: 'https://namnh198.github.io',
  base: '/lodev',
  markdown: {
    rehypePlugins: [
      [rehypeMdLinks, { base: '/lodev' }],
    ],
  },
  integrations: [
    starlight({
      title: 'lodev',
      logo: { src: './logo/lodev-logo.svg', replacesTitle: true },
      favicon: '/favicon.svg',
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/namnh198/lodev' }],
      sidebar: [
        { label: 'Getting started', items: [{ autogenerate: { directory: 'getting-started' } }] },
        { label: 'Configuration', items: [{ autogenerate: { directory: 'configuration' } }] },
        { label: 'Features', items: [{ autogenerate: { directory: 'features' } }] },
        { label: 'Services', items: [{ autogenerate: { directory: 'services' } }] },
        { label: 'Commands', link: '/commands/' },
        { label: 'Contributing', items: [{ autogenerate: { directory: 'contributing' } }] },
      ],
      editLink: { baseUrl: 'https://github.com/namnh198/lodev/edit/main/docs/' },
    }),
  ],
});
