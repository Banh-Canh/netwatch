// This module holds the shared state for the application frontend.

// We export the variable so it can be imported and modified by other modules.
export let allServices = []

// A function to update the state, ensuring consistency.
export function setAllServices(services) {
  allServices = services
}
