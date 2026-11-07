- Make monitor startup-cancellation coverage deterministic by blocking initial
  snapshot delivery until cancellation, independently of later full-queue
  shutdown coverage. The test verifies prompt return and connection cleanup.
