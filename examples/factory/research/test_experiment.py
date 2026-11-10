import unittest
from experiment import experiment

class ReproductionTest(unittest.TestCase):
    def test_recorded_inputs_reproduce_measurements(self):
        baseline = experiment()
        repeated = experiment(seed=baseline['seed'], regularisation=baseline['regularisation'])
        for key in ('dataset_sha256', 'held_out_mse', 'slope'):
            self.assertEqual(baseline[key], repeated[key])
        self.assertLess(baseline['held_out_mse'], 0.02)
        variation = experiment(regularisation=10)
        self.assertEqual(baseline['dataset_sha256'], variation['dataset_sha256'])
        self.assertLess(variation['slope'], baseline['slope'])
        self.assertNotEqual(experiment(seed=43)['dataset_sha256'], baseline['dataset_sha256'])

if __name__ == '__main__':
    unittest.main()
