import { mount } from 'svelte'
import App from './App.svelte'
import { System } from '@wailsio/runtime'

// On macOS the title bar is hidden and the page runs under it (main.go);
// style.css keeps content clear of the window buttons under .mac.
if (System.IsMac()) document.documentElement.classList.add('mac')

mount(App, { target: document.getElementById('app')! })
