package main

import "fyne.io/fyne/v2"

var (
	iconHash = fyne.NewStaticResource("icon-hash.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none">
  <path d="M8 3L6 21" stroke="#7CB342" stroke-width="2" stroke-linecap="round"/>
  <path d="M16 3L14 21" stroke="#7CB342" stroke-width="2" stroke-linecap="round"/>
  <path d="M4 9H20" stroke="#7CB342" stroke-width="2" stroke-linecap="round"/>
  <path d="M3 15H19" stroke="#7CB342" stroke-width="2" stroke-linecap="round"/>
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

