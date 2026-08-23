"""The Vekst classification service.

A separate Python service from day one (design D2). It holds no database
credentials and is never reachable from outside the cluster. Engine layers
L0-L2 arrive with change 3.2.
"""
