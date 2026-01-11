package main

import "fyne.io/fyne/v2"

var (
	iconHash = fyne.NewStaticResource("icon-pickaxe.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none">
  <g transform="rotate(35 12 12)">
    <path d="M4 8C6.8 4.2 10.2 3 12 3s5.2 1.2 8 5" stroke="#9CA3AF" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
    <path d="M12 4.5V8.5" stroke="#9CA3AF" stroke-width="2.2" stroke-linecap="round"/>
    <path d="M12 8.5V21" stroke="#B45309" stroke-width="2.4" stroke-linecap="round"/>
    <path d="M10.7 21H13.3" stroke="#8B5E34" stroke-width="2.4" stroke-linecap="round"/>
  </g>
</svg>`))
	iconPickaxeWhite = fyne.NewStaticResource("icon-pickaxe-white.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none">
  <g transform="rotate(35 12 12)">
    <path d="M4 8C6.8 4.2 10.2 3 12 3s5.2 1.2 8 5" stroke="#E5E7EB" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
    <path d="M12 4.5V8.5" stroke="#E5E7EB" stroke-width="2.2" stroke-linecap="round"/>
    <path d="M12 8.5V21" stroke="#E5E7EB" stroke-width="2.4" stroke-linecap="round"/>
    <path d="M10.7 21H13.3" stroke="#E5E7EB" stroke-width="2.4" stroke-linecap="round"/>
  </g>
</svg>`))
	iconThermometer = fyne.NewStaticResource("icon-thermometer.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none">
  <rect x="10" y="3" width="4" height="10" rx="2" fill="#F87171"/>
  <circle cx="12" cy="17" r="5" fill="#F87171"/>
</svg>`))
	iconFan = fyne.NewStaticResource("icon-fan.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none">
  <circle cx="12" cy="12" r="2.2" fill="#60A5FA"/>
  <path d="M12 4c3 0 4 2 4 4-2.5 0-4 0-4-4z" fill="#60A5FA"/>
  <path d="M20 12c0 3-2 4-4 4 0-2.5 0-4 4-4z" fill="#60A5FA"/>
  <path d="M12 20c-3 0-4-2-4-4 2.5 0 4 0 4 4z" fill="#60A5FA"/>
  <path d="M4 12c0-3 2-4 4-4 0 2.5 0 4-4 4z" fill="#60A5FA"/>
</svg>`))
	iconBolt = fyne.NewStaticResource("icon-bolt.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none">
  <path d="M13 2L5 13h5l-1 9 8-11h-5l1-9z" fill="#FACC15"/>
</svg>`))
)
