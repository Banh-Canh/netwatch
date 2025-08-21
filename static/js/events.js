// This module sets up all event listeners for the application.

import { showView, updateServiceDropdown } from './ui.js'

// Get only the elements needed for attaching events.
const elements = {
  gotoServiceAccessCard: document.getElementById('goto-service-access-card'),
  gotoExternalAccessCard: document.getElementById('goto-external-access-card'),
  gotoHubCard: document.getElementById('goto-hub-card'),
  backToMenuBtnSvc: document.getElementById('back-to-menu-btn-svc'),
  backToMenuBtnExt: document.getElementById('back-to-menu-btn-ext'),
  backToMenuBtnHub: document.getElementById('back-to-menu-btn-hub'),
  refreshServicesBtn: document.getElementById('refresh-services-btn'),
  refreshEaServicesBtn: document.getElementById('refresh-ea-services-btn'),
  refreshAccessBtnDashboard: document.getElementById(
    'refresh-access-btn-dashboard',
  ),
  refreshAccessBtnSvc: document.getElementById('refresh-access-btn-svc'),
  refreshAccessBtnExt: document.getElementById('refresh-access-btn-ext'),
  sourceNsFilter: document.getElementById('ca-source-ns-filter'),
  targetNsFilter: document.getElementById('ca-target-ns-filter'),
  eaNsFilter: document.getElementById('ea-ns-filter'),
  caForm: document.getElementById('cluster-access-form'),
  eaForm: document.getElementById('external-access-form'),
}

export function setupEventListeners(socket, handlers) {
  // --- Navigation ---
  elements.gotoServiceAccessCard.addEventListener('click', () => {
    showView('service-access-view')
    handlers.initializeCaFilters()
  })
  elements.gotoExternalAccessCard.addEventListener('click', () => {
    showView('external-access-view')
    handlers.initializeEaFilters()
  })
  elements.gotoHubCard.addEventListener('click', () => {
    showView('access-request-hub-view')
    handlers.fetchAndRenderPendingRequests()
  })
  ;[
    elements.backToMenuBtnSvc,
    elements.backToMenuBtnExt,
    elements.backToMenuBtnHub,
  ].forEach((btn) => {
    btn.addEventListener('click', (e) => {
      e.preventDefault()
      showView('main-menu-view')
    })
  })

  // --- Form Submissions ---
  document.getElementById('ca-create-btn').addEventListener('click', () => {
    if (!elements.caForm.checkValidity()) {
      elements.caForm.reportValidity()
      return
    }
    socket.send(
      JSON.stringify({
        command: 'requestClusterAccess',
        sourceService: document.getElementById('ca-source-svc').value,
        targetService: document.getElementById('ca-target-svc').value,
        duration: parseInt(document.getElementById('ca-duration').value, 10),
        direction: document.getElementById('ca-direction').value,
        ports: document.getElementById('ca-ports').value,
      }),
    )
  })

  document
    .getElementById('ca-submit-review-btn')
    .addEventListener('click', () => {
      if (!elements.caForm.checkValidity()) {
        elements.caForm.reportValidity()
        return
      }
      socket.send(
        JSON.stringify({
          command: 'submitAccessRequest',
          sourceService: document.getElementById('ca-source-svc').value,
          targetService: document.getElementById('ca-target-svc').value,
          duration: parseInt(document.getElementById('ca-duration').value, 10),
          direction: document.getElementById('ca-direction').value,
          ports: document.getElementById('ca-ports').value,
          description: document.getElementById('ca-description').value,
        }),
      )
    })

  document.getElementById('ea-create-btn').addEventListener('click', () => {
    if (!elements.eaForm.checkValidity()) {
      elements.eaForm.reportValidity()
      return
    }
    socket.send(
      JSON.stringify({
        command: 'requestExternalAccess',
        cidr: document.getElementById('ea-cidr').value,
        service: document.getElementById('ea-service').value,
        duration: parseInt(document.getElementById('ea-duration').value, 10),
        direction: document.getElementById('ea-direction').value,
        ports: document.getElementById('ea-ports').value,
      }),
    )
  })

  document
    .getElementById('ea-submit-review-btn')
    .addEventListener('click', () => {
      if (!elements.eaForm.checkValidity()) {
        elements.eaForm.reportValidity()
        return
      }
      socket.send(
        JSON.stringify({
          command: 'submitAccessRequest',
          cidr: document.getElementById('ea-cidr').value,
          service: document.getElementById('ea-service').value,
          duration: parseInt(document.getElementById('ea-duration').value, 10),
          direction: document.getElementById('ea-direction').value,
          ports: document.getElementById('ea-ports').value,
          description: document.getElementById('ea-description').value,
        }),
      )
    })

  // --- Dynamic Content Buttons (using event delegation) ---
  document.body.addEventListener('click', (event) => {
    const revokeBtn = event.target.closest('.revoke-btn')
    if (revokeBtn) {
      if (confirm('Are you sure you want to revoke this access?')) {
        const row = revokeBtn.closest('tr')
        if (row) {
          row.style.opacity = '0.5'
          row.style.pointerEvents = 'none'
        }
        revokeBtn.disabled = true

        socket.send(
          JSON.stringify({
            command:
              revokeBtn.dataset.type === 'Service'
                ? 'revokeClusterAccess'
                : 'revokeExternalAccess',
            name: revokeBtn.dataset.name,
            namespace: revokeBtn.dataset.namespace,
          }),
        )
      }
    }

    const viewRequestBtn = event.target.closest('.view-request-btn')
    if (viewRequestBtn) {
      showView('access-request-hub-view')
      handlers.fetchAndRenderPendingRequests()
    }

    const approveBtn = event.target.closest('.approve-btn')
    if (approveBtn) {
      if (confirm('Are you sure you want to approve this access request?')) {
        approveBtn.disabled = true
        socket.send(
          JSON.stringify({
            command: 'approveAccessRequest',
            requestID: approveBtn.dataset.id,
          }),
        )
      }
    }

    const denyBtn = event.target.closest('.deny-btn')
    if (denyBtn) {
      const confirmText =
        denyBtn.textContent.trim() === 'Abort'
          ? 'Are you sure you want to abort your access request?'
          : 'Are you sure you want to deny this access request?'

      if (confirm(confirmText)) {
        denyBtn.disabled = true
        socket.send(
          JSON.stringify({
            command: 'denyAccessRequest',
            requestID: denyBtn.dataset.id,
          }),
        )
      }
    }
  })

  // --- Refresh Buttons ---
  elements.refreshServicesBtn.addEventListener(
    'click',
    handlers.initializeCaFilters,
  )
  elements.refreshEaServicesBtn.addEventListener(
    'click',
    handlers.initializeEaFilters,
  )
  ;[
    elements.refreshAccessBtnDashboard,
    elements.refreshAccessBtnSvc,
    elements.refreshAccessBtnExt,
  ].forEach((btn) => {
    btn.addEventListener('click', handlers.fetchAndDisplayActiveAccesses)
  })

  // --- Filter Dropdowns ---
  elements.sourceNsFilter.addEventListener('change', (event) => {
    updateServiceDropdown(
      document.getElementById('ca-source-svc'),
      event.target.value,
    )
  })
  elements.targetNsFilter.addEventListener('change', (event) => {
    updateServiceDropdown(
      document.getElementById('ca-target-svc'),
      event.target.value,
    )
  })
  elements.eaNsFilter.addEventListener('change', (event) => {
    updateServiceDropdown(
      document.getElementById('ea-service'),
      event.target.value,
    )
  })
}
